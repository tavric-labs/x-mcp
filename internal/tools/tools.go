// Package tools implements the X MCP tools. A tool call carries the caller's
// mcp-auth access token; the server exchanges it for that person's own X token
// and calls X as them. No X token is stored here.
package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tavric-labs/x-mcp/internal/x"
)

// SubjectTokenKey is the TokenInfo.Extra key under which the request's verified
// mcp-auth access token is carried to the tools. It is read only to exchange it
// and is never logged or returned.
const SubjectTokenKey = "mcp_subject_token"

// WriteScope is the OAuth scope a token needs to publish. A token with only the
// read scope can preview a post but not send it.
const WriteScope = "tools:write"

// MaxPostRunes is the standard X post length for accounts without longer-post
// entitlement. Premium limits can be raised later if needed.
const MaxPostRunes = 280

// XAPI is the part of the X API the tools use.
type XAPI interface {
	Profile(ctx context.Context, token string) (x.Profile, error)
	CreatePost(ctx context.Context, token, text string) (string, error)
}

// TokenSource turns the caller's mcp-auth access token into that person's
// X access token.
type TokenSource interface {
	MemberToken(ctx context.Context, subjectToken string) (string, error)
}

// Server holds the tool dependencies.
type Server struct {
	X      XAPI
	Tokens TokenSource
}

// ProfileArgs has no inputs.
type ProfileArgs struct{}

// PostArgs are the inputs of x_create_post.
type PostArgs struct {
	Text    string `json:"text" jsonschema:"the post text, up to 280 characters"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"set true to publish; without it the call is a dry run that publishes nothing"`
}

// GetProfile returns the signed-in person's X profile.
func (s *Server) GetProfile(ctx context.Context, request *mcp.CallToolRequest, _ ProfileArgs) (*mcp.CallToolResult, any, error) {
	subject, token, failure := s.memberToken(ctx, request)
	if failure != nil {
		return failure, nil, nil
	}
	profile, err := s.X.Profile(ctx, token)
	if err != nil {
		return s.xFailure("x_get_profile", subject, err), nil, nil
	}
	slog.Info("tool call", "tool", "x_get_profile", "subject", subject, "outcome", "success")
	return result(fmt.Sprintf("Signed in to X as @%s (%s, id %s).", profile.Username, profile.Name, profile.ID), false), nil, nil
}

// CreatePost publishes a post as the signed-in person, or previews it.
func (s *Server) CreatePost(ctx context.Context, request *mcp.CallToolRequest, args PostArgs) (*mcp.CallToolResult, any, error) {
	text := strings.TrimSpace(args.Text)
	if text == "" {
		return result("The post text is empty.", true), nil, nil
	}
	if utf8.RuneCountInString(text) > MaxPostRunes {
		return result(fmt.Sprintf("The post is longer than X's %d characters.", MaxPostRunes), true), nil, nil
	}
	if !args.Confirm {
		return result(fmt.Sprintf("Dry run, nothing was posted. This would publish to X:\n\n%s\n\nCall again with confirm: true to publish.", text), false), nil, nil
	}
	if !hasScope(request, WriteScope) {
		return result("Publishing needs the "+WriteScope+" scope. Reconnect with write access.", true), nil, nil
	}
	subject, token, failure := s.memberToken(ctx, request)
	if failure != nil {
		return failure, nil, nil
	}
	profile, err := s.X.Profile(ctx, token)
	if err != nil {
		return s.xFailure("x_create_post", subject, err), nil, nil
	}
	id, err := s.X.CreatePost(ctx, token, text)
	if err != nil {
		return s.xFailure("x_create_post", subject, err), nil, nil
	}
	slog.Info("tool call", "tool", "x_create_post", "subject", subject, "outcome", "success", "post", id, "characters", utf8.RuneCountInString(text))
	return result(fmt.Sprintf("Posted to X as @%s: https://x.com/%s/status/%s", profile.Username, profile.Username, id), false), nil, nil
}

// memberToken exchanges the request's mcp-auth token for the person's X token.
// On failure it returns a ready tool error result.
func (s *Server) memberToken(ctx context.Context, request *mcp.CallToolRequest) (subject, token string, failure *mcp.CallToolResult) {
	var info map[string]any
	if request != nil && request.Extra != nil && request.Extra.TokenInfo != nil {
		subject = request.Extra.TokenInfo.UserID
		info = request.Extra.TokenInfo.Extra
	}
	subjectToken, _ := info[SubjectTokenKey].(string)
	if subjectToken == "" {
		return subject, "", result("This call is not authenticated.", true)
	}
	token, err := s.Tokens.MemberToken(ctx, subjectToken)
	if err != nil {
		slog.Warn("x token exchange failed", "subject", subject, "error", err.Error())
		return subject, "", result("Could not get your X session. Sign in again from your MCP client.", true)
	}
	return subject, token, nil
}

func hasScope(request *mcp.CallToolRequest, scope string) bool {
	if request == nil || request.Extra == nil || request.Extra.TokenInfo == nil {
		return false
	}
	for _, have := range request.Extra.TokenInfo.Scopes {
		if have == scope {
			return true
		}
	}
	return false
}

func (s *Server) xFailure(tool, subject string, err error) *mcp.CallToolResult {
	slog.Warn("tool call failed", "tool", tool, "subject", subject, "error", err.Error())
	switch {
	case errors.Is(err, x.ErrUnauthorized):
		return result("Your X session has expired. Sign in again from your MCP client.", true)
	case errors.Is(err, x.ErrPaymentRequired):
		return result("X API credits are required. Add credits in the X Developer Console, then try again.", true)
	}
	return result("X could not complete the request.", true)
}

func result(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
