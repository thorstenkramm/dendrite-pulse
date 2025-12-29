package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMaxUploadSize(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    int64
		wantErr bool
	}{
		{"bytes", "1024", 1024, false},
		{"gigabytes", "2GB", 2 * 1024 * 1024 * 1024, false},
		{"empty rejected", "", 0, true},
		{"invalid rejected", "bad-size", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMaxUploadSize(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseFileMode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    uint32
		wantErr bool
	}{
		{"valid octal", "0600", 0o600, false},
		{"zero allowed", "0000", 0, false},
		{"empty rejected", "", 0, true},
		{"invalid rejected", "not-octal", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFileMode(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, uint32(got))
		})
	}
}
