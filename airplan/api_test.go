package airplan

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClientOperationsRejectUninitializedClient(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "upload",
			call: func(client *Client) error {
				_, err := client.Upload(context.Background(), Input{})
				return err
			},
		},
		{
			name: "list remote",
			call: func(client *Client) error {
				_, err := client.ListRemote(context.Background())
				return err
			},
		},
		{
			name: "inspect upload",
			call: func(client *Client) error {
				_, err := client.InspectUpload(context.Background(), "key")
				return err
			},
		},
		{
			name: "delete upload",
			call: func(client *Client) error {
				_, err := client.DeleteUpload(context.Background(), "key")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" on zero value", func(t *testing.T) {
			if err := tt.call(&Client{}); !errors.Is(err, ErrUninitializedClient) {
				t.Fatalf("error = %v, want ErrUninitializedClient", err)
			}
		})
		t.Run(tt.name+" on nil receiver", func(t *testing.T) {
			if err := tt.call(nil); !errors.Is(err, ErrUninitializedClient) {
				t.Fatalf("error = %v, want ErrUninitializedClient", err)
			}
		})
	}
}

func TestPublicOperationsRejectNilContext(t *testing.T) {
	//nolint:staticcheck // The public boundary must reject a nil context.
	if _, err := New(nil, &Config{}); err == nil ||
		!strings.Contains(err.Error(), "nil context") {
		t.Fatalf("New(nil) error = %v", err)
	}
	//nolint:staticcheck // The public boundary must reject a nil context.
	if _, err := RenderInput(nil, Input{}, RenderInputOptions{}); err == nil ||
		!strings.Contains(err.Error(), "nil context") {
		t.Fatalf("RenderInput(nil) error = %v", err)
	}

	client := &Client{cfg: &Config{}, st: &storage{}}
	tests := []struct {
		name string
		call func() error
	}{
		{"upload", func() error {
			//nolint:staticcheck // The public boundary must reject a nil context.
			_, err := client.Upload(nil, Input{})
			return err
		}},
		{"list remote", func() error {
			//nolint:staticcheck // The public boundary must reject a nil context.
			_, err := client.ListRemote(nil)
			return err
		}},
		{"inspect upload", func() error {
			//nolint:staticcheck // The public boundary must reject a nil context.
			_, err := client.InspectUpload(nil, "key")
			return err
		}},
		{"delete upload", func() error {
			//nolint:staticcheck // The public boundary must reject a nil context.
			_, err := client.DeleteUpload(nil, "key")
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil ||
				!strings.Contains(err.Error(), "nil context") {
				t.Fatalf("error = %v, want nil context", err)
			}
		})
	}
}
