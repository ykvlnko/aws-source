package elbv2

import (
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

func ActionToRequests(action types.Action) []*sdp.ItemRequest {
	requests := make([]*sdp.ItemRequest, 0)

	if action.AuthenticateCognitoConfig != nil {
		if action.AuthenticateCognitoConfig.UserPoolArn != nil {
			if a, err := sources.ParseARN(*action.AuthenticateCognitoConfig.UserPoolArn); err == nil {
				requests = append(requests, &sdp.ItemRequest{
					Type:   "cognito-idp-user-pool",
					Method: sdp.RequestMethod_SEARCH,
					Query:  *action.AuthenticateCognitoConfig.UserPoolArn,
					Scope:  sources.FormatScope(a.AccountID, a.Region),
				})
			}
		}
	}

	if action.AuthenticateOidcConfig != nil {
		if action.AuthenticateOidcConfig.AuthorizationEndpoint != nil {
			requests = append(requests, &sdp.ItemRequest{
				Type:   "http",
				Method: sdp.RequestMethod_GET,
				Query:  *action.AuthenticateOidcConfig.AuthorizationEndpoint,
				Scope:  "global",
			})
		}

		if action.AuthenticateOidcConfig.TokenEndpoint != nil {
			requests = append(requests, &sdp.ItemRequest{
				Type:   "http",
				Method: sdp.RequestMethod_GET,
				Query:  *action.AuthenticateOidcConfig.TokenEndpoint,
				Scope:  "global",
			})
		}

		if action.AuthenticateOidcConfig.UserInfoEndpoint != nil {
			requests = append(requests, &sdp.ItemRequest{
				Type:   "http",
				Method: sdp.RequestMethod_GET,
				Query:  *action.AuthenticateOidcConfig.UserInfoEndpoint,
				Scope:  "global",
			})
		}

		if action.ForwardConfig != nil {
			for _, tg := range action.ForwardConfig.TargetGroups {
				if tg.TargetGroupArn != nil {
					if a, err := sources.ParseARN(*tg.TargetGroupArn); err == nil {
						requests = append(requests, &sdp.ItemRequest{
							Type:   "elbv2-target-group",
							Method: sdp.RequestMethod_SEARCH,
							Query:  *tg.TargetGroupArn,
							Scope:  sources.FormatScope(a.AccountID, a.Region),
						})
					}
				}
			}
		}

		if action.RedirectConfig != nil {
			u := url.URL{}

			if action.RedirectConfig.Path != nil {
				u.Path = *action.RedirectConfig.Path
			}

			if action.RedirectConfig.Port != nil {
				u.Port()
			}

			if action.RedirectConfig.Host != nil {
				u.Host = *action.RedirectConfig.Host

				if action.RedirectConfig.Port != nil {
					u.Host = u.Host + fmt.Sprintf(":%v", *action.RedirectConfig.Port)
				}
			}

			if action.RedirectConfig.Protocol != nil {
				u.Scheme = *action.RedirectConfig.Protocol
			}

			if action.RedirectConfig.Query != nil {
				u.RawQuery = *action.RedirectConfig.Query
			}

			if u.Scheme == "http" || u.Scheme == "https" {
				requests = append(requests, &sdp.ItemRequest{
					Type:   "http",
					Method: sdp.RequestMethod_GET,
					Query:  u.String(),
					Scope:  "global",
				})
			}
		}

		if action.TargetGroupArn != nil {
			if a, err := sources.ParseARN(*action.TargetGroupArn); err == nil {
				requests = append(requests, &sdp.ItemRequest{
					Type:   "elbv2-target-group",
					Method: sdp.RequestMethod_SEARCH,
					Query:  *action.TargetGroupArn,
					Scope:  sources.FormatScope(a.AccountID, a.Region),
				})
			}
		}
	}

	return requests
}
