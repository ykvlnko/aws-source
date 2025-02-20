package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/overmindtech/sdp-go"
	"github.com/overmindtech/sdpcache"
)

const DefaultCacheDuration = 1 * time.Hour

// DescribeOnlySource Generates a source for AWS APIs that only use a `Describe`
// function for both List and Get operations. EC2 is a good example of this,
// where running Describe with no params returns everything, but params can be
// supplied to reduce the number of results.
type DescribeOnlySource[Input InputType, Output OutputType, ClientStruct ClientStructType, Options OptionsType] struct {
	MaxResultsPerPage int32  // Max results per page when making API queries
	ItemType          string // The type of items that will be returned

	CacheDuration time.Duration   // How long to cache items for
	cache         *sdpcache.Cache // The sdpcache of this source
	cacheInitMu   sync.Mutex      // Mutex to ensure cache is only initialised once

	// The function that should be used to describe the resources that this
	// source is related to
	DescribeFunc func(ctx context.Context, client ClientStruct, input Input) (Output, error)

	// A function that returns the input object that will be passed to
	// DescribeFunc for a GET request
	InputMapperGet func(scope, query string) (Input, error)

	// A function that returns the input object that will be passed to
	// DescribeFunc for a LIST request
	InputMapperList func(scope string) (Input, error)

	// A function that maps a search query to the required input. If this is
	// unset then a search request will default to searching by ARN
	InputMapperSearch func(ctx context.Context, client ClientStruct, scope string, query string) (Input, error)

	// A function that returns a paginator for this API. If this is nil, we will
	// assume that the API is not paginated e.g.
	// https://aws.github.io/aws-sdk-go-v2/docs/making-requests/#using-paginators
	PaginatorBuilder func(client ClientStruct, params Input) Paginator[Output, Options]

	// A function that returns a slice of items for a given output. The scope
	// and input are passed in on order to assist in creating the items if
	// needed, but primarily this function should iterate over the output and
	// create new items for each result
	OutputMapper func(ctx context.Context, client ClientStruct, scope string, input Input, output Output) ([]*sdp.Item, error)

	// Config AWS Config including region and credentials
	Config aws.Config

	// AccountID The id of the account that is being used. This is used by
	// sources as the first element in the scope
	AccountID string

	// Client The AWS client to use when making requests
	Client ClientStruct

	// UseListForGet If true, the source will use the List function to get items
	// This option should be used when the Describe function does not support
	// getting a single item by ID. The source will then filter the items
	// itself.
	// InputMapperGet should still be defined. It will be used to create the
	// input for the List function. The output of the List function will be
	// filtered by the source to find the item with the matching ID.
	// See the directconnect-virtual-gateway source for an example of this.
	UseListForGet bool
}

// Returns the duration that items should be cached for. This will use the
// `CacheDuration` for this source if set, otherwise it will use the default
// duration of 1 hour
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) cacheDuration() time.Duration {
	if s.CacheDuration == 0 {
		return DefaultCacheDuration
	}

	return s.CacheDuration
}

func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) ensureCache() {
	s.cacheInitMu.Lock()
	defer s.cacheInitMu.Unlock()

	if s.cache == nil {
		s.cache = sdpcache.NewCache()
	}
}

func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Cache() *sdpcache.Cache {
	s.ensureCache()
	return s.cache
}

// Validate Checks that the source is correctly set up and returns an error if
// not
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Validate() error {
	if s.DescribeFunc == nil {
		return errors.New("source describe func is nil")
	}

	if s.MaxResultsPerPage == 0 {
		s.MaxResultsPerPage = DefaultMaxResultsPerPage
	}

	if s.InputMapperGet == nil {
		return errors.New("source get input mapper is nil")
	}

	if s.OutputMapper == nil {
		return errors.New("source output mapper is nil")
	}

	return nil
}

// Paginated returns whether or not this source is using a paginated API
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Paginated() bool {
	return s.PaginatorBuilder != nil
}

func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Type() string {
	return s.ItemType
}

func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Name() string {
	return fmt.Sprintf("%v-source", s.ItemType)
}

// List of scopes that this source is capable of find items for. This will be
// in the format {accountID}.{region}
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Scopes() []string {
	return []string{
		FormatScope(s.AccountID, s.Config.Region),
	}
}

// Get Get a single item with a given scope and query. The item returned
// should have a UniqueAttributeValue that matches the `query` parameter. The
// ctx parameter contains a golang context object which should be used to allow
// this source to timeout or be cancelled when executing potentially
// long-running actions
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Get(ctx context.Context, scope string, query string, ignoreCache bool) (*sdp.Item, error) {
	if scope != s.Scopes()[0] {
		return nil, &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOSCOPE,
			ErrorString: fmt.Sprintf("requested scope %v does not match source scope %v", scope, s.Scopes()[0]),
		}
	}

	var input Input
	var output Output
	var err error
	var items []*sdp.Item

	err = s.Validate()
	if err != nil {
		return nil, WrapAWSError(err)
	}

	s.ensureCache()
	cacheHit, ck, cachedItems, qErr := s.cache.Lookup(ctx, s.Name(), sdp.QueryMethod_GET, scope, s.ItemType, query, ignoreCache)
	if qErr != nil {
		return nil, qErr
	}
	if cacheHit {
		if len(cachedItems) > 0 {
			return cachedItems[0], nil
		} else {
			return nil, nil
		}
	}

	// Get the input object
	input, err = s.InputMapperGet(scope, query)
	if err != nil {
		err = WrapAWSError(err)
		s.cache.StoreError(err, s.cacheDuration(), ck)
		return nil, err
	}

	// Call the API using the object
	output, err = s.DescribeFunc(ctx, s.Client, input)
	if err != nil {
		err = WrapAWSError(err)
		s.cache.StoreError(err, s.cacheDuration(), ck)
		return nil, err
	}

	items, err = s.OutputMapper(ctx, s.Client, scope, input, output)
	if err != nil {
		err = WrapAWSError(err)
		s.cache.StoreError(err, s.cacheDuration(), ck)
		return nil, err
	}

	if s.UseListForGet {
		// If we're using List for Get, we need to filter the items ourselves
		var filteredItems []*sdp.Item
		for _, item := range items {
			if item.UniqueAttributeValue() == query {
				filteredItems = append(filteredItems, item)
				break
			}
		}
		items = filteredItems
	}

	numItems := len(items)

	switch {
	case numItems > 1:
		itemNames := make([]string, len(items))

		// Get the names for logging
		for i := range items {
			itemNames[i] = items[i].GloballyUniqueName()
		}

		qErr := &sdp.QueryError{
			ErrorType:   sdp.QueryError_OTHER,
			ErrorString: fmt.Sprintf("Request returned > 1 item for a GET request. Items: %v", strings.Join(itemNames, ", ")),
		}
		s.cache.StoreError(qErr, s.cacheDuration(), ck)

		return nil, qErr
	case numItems == 0:
		qErr := &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOTFOUND,
			ErrorString: fmt.Sprintf("%v %v not found", s.Type(), query),
		}
		s.cache.StoreError(qErr, s.cacheDuration(), ck)
		return nil, qErr
	}

	s.cache.StoreItem(items[0], s.cacheDuration(), ck)
	return items[0], nil
}

// List Lists all items in a given scope
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) List(ctx context.Context, scope string, ignoreCache bool) ([]*sdp.Item, error) {
	if scope != s.Scopes()[0] {
		return nil, &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOSCOPE,
			ErrorString: fmt.Sprintf("requested scope %v does not match source scope %v", scope, s.Scopes()[0]),
		}
	}

	if s.InputMapperList == nil {
		return nil, &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOTFOUND,
			ErrorString: fmt.Sprintf("list is not supported for %v resources", s.ItemType),
		}
	}

	err := s.Validate()
	if err != nil {
		return nil, WrapAWSError(err)
	}

	s.ensureCache()
	cacheHit, ck, cachedItems, qErr := s.cache.Lookup(ctx, s.Name(), sdp.QueryMethod_LIST, scope, s.ItemType, "", ignoreCache)
	if qErr != nil {
		return nil, qErr
	}
	if cacheHit {
		return cachedItems, nil
	}

	var items []*sdp.Item

	input, err := s.InputMapperList(scope)
	if err != nil {
		err = WrapAWSError(err)
		s.cache.StoreError(err, s.cacheDuration(), ck)
		return nil, err
	}

	items, err = s.describe(ctx, input, scope)
	if err != nil {
		err = WrapAWSError(err)
		s.cache.StoreError(err, s.cacheDuration(), ck)
		return nil, err
	}

	for _, item := range items {
		s.cache.StoreItem(item, s.cacheDuration(), ck)
	}

	return items, nil
}

// Search Searches for AWS resources by ARN
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Search(ctx context.Context, scope string, query string, ignoreCache bool) ([]*sdp.Item, error) {
	if scope != s.Scopes()[0] {
		return nil, &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOSCOPE,
			ErrorString: fmt.Sprintf("requested scope %v does not match source scope %v", scope, s.Scopes()[0]),
		}
	}

	ck := sdpcache.CacheKeyFromParts(s.Name(), sdp.QueryMethod_SEARCH, scope, s.ItemType, query)

	if s.InputMapperSearch == nil {
		return s.searchARN(ctx, scope, query, ignoreCache)
	} else {
		return s.searchCustom(ctx, scope, query, ck)
	}
}

func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) searchARN(ctx context.Context, scope string, query string, ignoreCache bool) ([]*sdp.Item, error) {
	// Parse the ARN
	a, err := ParseARN(query)

	if err != nil {
		return nil, WrapAWSError(err)
	}

	if arnScope := FormatScope(a.AccountID, a.Region); arnScope != scope {
		return nil, &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOSCOPE,
			ErrorString: fmt.Sprintf("ARN scope %v does not match request scope %v", arnScope, scope),
			Scope:       scope,
		}
	}

	// this already uses the cache, so needs no extra handling
	item, err := s.Get(ctx, scope, a.ResourceID(), ignoreCache)
	if err != nil {
		return nil, WrapAWSError(err)
	}

	return []*sdp.Item{item}, nil
}

// searchCustom Runs custom search logic using the `InputMapperSearch` function
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) searchCustom(ctx context.Context, scope string, query string, ck sdpcache.CacheKey) ([]*sdp.Item, error) {
	input, err := s.InputMapperSearch(ctx, s.Client, scope, query)
	if err != nil {
		return nil, WrapAWSError(err)
	}

	items, err := s.describe(ctx, input, scope)
	if err != nil {
		err = WrapAWSError(err)
		s.cache.StoreError(err, s.cacheDuration(), ck)
		return nil, err
	}

	for _, item := range items {
		s.cache.StoreItem(item, s.cacheDuration(), ck)
	}

	return items, nil
}

// describe Runs describe on the given input, intelligently choosing whether to
// run the paginated or unpaginated query
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) describe(ctx context.Context, input Input, scope string) ([]*sdp.Item, error) {
	var output Output
	var err error
	var newItems []*sdp.Item

	items := make([]*sdp.Item, 0)

	if s.Paginated() {
		paginator := s.PaginatorBuilder(s.Client, input)

		for paginator.HasMorePages() {
			output, err = paginator.NextPage(ctx)
			if err != nil {
				return nil, err
			}

			newItems, err = s.OutputMapper(ctx, s.Client, scope, input, output)
			if err != nil {
				return nil, err
			}

			items = append(items, newItems...)
		}
	} else {
		output, err = s.DescribeFunc(ctx, s.Client, input)
		if err != nil {
			return nil, err
		}

		items, err = s.OutputMapper(ctx, s.Client, scope, input, output)
		if err != nil {
			return nil, err
		}
	}

	return items, nil
}

// Weight Returns the priority weighting of items returned by this source.
// This is used to resolve conflicts where two sources of the same type
// return an item for a GET request. In this instance only one item can be
// seen on, so the one with the higher weight value will win.
func (s *DescribeOnlySource[Input, Output, ClientStruct, Options]) Weight() int {
	return 100
}
