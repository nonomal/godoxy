package rules

import (
	"testing"

	"github.com/yusing/go-proxy/internal/gperr"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestSplitAnd(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty",
			input: "",
			want:  []string{},
		},
		{
			name:  "single",
			input: "rule",
			want:  []string{"rule"},
		},
		{
			name:  "multiple",
			input: "rule1 & rule2",
			want:  []string{"rule1", "rule2"},
		},
		{
			name:  "multiple_newline",
			input: "rule1\n\nrule2",
			want:  []string{"rule1", "rule2"},
		},
		{
			name:  "multiple_newline_and",
			input: "rule1\nrule2 & rule3",
			want:  []string{"rule1", "rule2", "rule3"},
		},
		{
			name:  "empty segment",
			input: "rule1\n& &rule2& rule3",
			want:  []string{"rule1", "rule2", "rule3"},
		},
		{
			name:  "double_and",
			input: "rule1\nrule2 && rule3",
			want:  []string{"rule1", "rule2", "rule3"},
		},
		{
			name:  "spaces_around",
			input: " rule1\nrule2 & rule3 ",
			want:  []string{"rule1", "rule2", "rule3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAnd(tt.input)
			expect.Equal(t, got, tt.want)
		})
	}
}

func TestParseOn(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr gperr.Error
	}{
		// header
		{
			name:    "header_valid_kv",
			input:   "header Connection Upgrade",
			wantErr: nil,
		},
		{
			name:    "header_valid_k",
			input:   "header Connection",
			wantErr: nil,
		},
		{
			name:    "header_missing_arg",
			input:   "header",
			wantErr: ErrExpectKVOptionalV,
		},
		// query
		{
			name:    "query_valid_kv",
			input:   "query key value",
			wantErr: nil,
		},
		{
			name:    "query_valid_k",
			input:   "query key",
			wantErr: nil,
		},
		{
			name:    "query_missing_arg",
			input:   "query",
			wantErr: ErrExpectKVOptionalV,
		},
		{
			name:    "cookie_valid_kv",
			input:   "cookie key value",
			wantErr: nil,
		},
		{
			name:    "cookie_valid_k",
			input:   "cookie key",
			wantErr: nil,
		},
		{
			name:    "cookie_missing_arg",
			input:   "cookie",
			wantErr: ErrExpectKVOptionalV,
		},
		// method
		{
			name:    "method_valid",
			input:   "method GET",
			wantErr: nil,
		},
		{
			name:    "method_invalid",
			input:   "method invalid",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "method_missing_arg",
			input:   "method",
			wantErr: ErrExpectOneArg,
		},
		// path
		{
			name:    "path_valid",
			input:   "path /home",
			wantErr: nil,
		},
		{
			name:    "path_missing_arg",
			input:   "path",
			wantErr: ErrExpectOneArg,
		},
		// remote
		{
			name:    "remote_valid",
			input:   "remote 127.0.0.1",
			wantErr: nil,
		},
		{
			name:    "remote_invalid",
			input:   "remote abcd",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "remote_missing_arg",
			input:   "remote",
			wantErr: ErrExpectOneArg,
		},
		{
			name:    "unknown_target",
			input:   "unknown",
			wantErr: ErrInvalidOnTarget,
		},
		// route
		{
			name:    "route_valid",
			input:   "route example",
			wantErr: nil,
		},
		{
			name:    "route_missing_arg",
			input:   "route",
			wantErr: ErrExpectOneArg,
		},
		{
			name:    "route_extra_arg",
			input:   "route example1 example2",
			wantErr: ErrExpectOneArg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			on := &RuleOn{}
			err := on.Parse(tt.input)
			if tt.wantErr != nil {
				expect.HasError(t, tt.wantErr, err)
			} else {
				expect.NoError(t, err)
			}
		})
	}
}
