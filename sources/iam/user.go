package iam

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"

	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

type UserDetails struct {
	User       *types.User
	UserGroups []types.Group
}

type IAMClient interface {
	ListGroupsForUser(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error)
	GetUser(ctx context.Context, params *iam.GetUserInput, optFns ...func(*iam.Options)) (*iam.GetUserOutput, error)
	ListUsers(ctx context.Context, params *iam.ListUsersInput, optFns ...func(*iam.Options)) (*iam.ListUsersOutput, error)
}

func UserGetFunc(ctx context.Context, client IAMClient, scope, query string) (*UserDetails, error) {
	out, err := client.GetUser(ctx, &iam.GetUserInput{
		UserName: &query,
	})

	if err != nil {
		return nil, err
	}

	details := UserDetails{
		User: out.User,
	}

	if out.User != nil {
		// Get the groups that the user is in too soe that we can create linked item requests
		groups, err := GetUserGroups(ctx, client, out.User.UserName)

		if err == nil {
			details.UserGroups = groups
		}
	}

	return &details, nil
}

// Gets all of the groups that a user is in
func GetUserGroups(ctx context.Context, client IAMClient, userName *string) ([]types.Group, error) {
	var out *iam.ListGroupsForUserOutput
	var marker *string
	var err error
	truncated := true
	groups := make([]types.Group, 0)

	for truncated {
		out, err = client.ListGroupsForUser(ctx, &iam.ListGroupsForUserInput{
			UserName: userName,
			Marker:   marker,
		})

		if err == nil {
			marker = out.Marker
			truncated = out.IsTruncated

			groups = append(groups, out.Groups...)
		} else {
			return nil, err
		}
	}

	return groups, nil
}

func UserListFunc(ctx context.Context, client IAMClient, scope string) ([]*UserDetails, error) {
	var out *iam.ListUsersOutput
	var err error
	var marker *string
	isTruncated := true
	users := make([]types.User, 0)

	for isTruncated {
		out, err = client.ListUsers(ctx, &iam.ListUsersInput{
			Marker: marker,
		})

		if err != nil {
			return nil, err
		}

		isTruncated = out.IsTruncated
		marker = out.Marker
		users = append(users, out.Users...)
	}

	userDetails := make([]*UserDetails, len(users))

	for i, user := range users {
		details := UserDetails{
			User: &user,
		}

		groups, err := GetUserGroups(ctx, client, user.UserName)

		if err == nil {
			details.UserGroups = groups
		}

		userDetails[i] = &details
	}

	return userDetails, nil
}

func UserItemMapper(scope string, awsItem *UserDetails) (*sdp.Item, error) {
	attributes, err := sources.ToAttributesCase(awsItem.User)

	if err != nil {
		return nil, err
	}

	item := sdp.Item{
		Type:            "iam-user",
		UniqueAttribute: "userName",
		Attributes:      attributes,
		Scope:           scope,
	}

	for _, group := range awsItem.UserGroups {
		item.LinkedItemRequests = append(item.LinkedItemRequests, &sdp.ItemRequest{
			Type:   "iam-group",
			Method: sdp.RequestMethod_GET,
			Query:  *group.GroupName,
			Scope:  scope,
		})
	}

	return &item, nil
}

func NewUserSource(config aws.Config, accountID string, region string) *sources.GetListSource[*UserDetails, IAMClient, *iam.Options] {
	return &sources.GetListSource[*UserDetails, IAMClient, *iam.Options]{
		ItemType:   "iam-user",
		Client:     iam.NewFromConfig(config),
		AccountID:  accountID,
		Region:     region,
		GetFunc:    UserGetFunc,
		ListFunc:   UserListFunc,
		ItemMapper: UserItemMapper,
	}
}
