package main

import (
	"testing"

	"github.com/dipdup-io/starknet-go-api/pkg/data"
	"github.com/stretchr/testify/require"
)

func Test_parseStringArray(t *testing.T) {
	tests := []struct {
		name       string
		response   []data.Felt
		want       string
		wantOffset int
	}{
		{
			name: "test 1",
			response: []data.Felt{
				"0x0", "0x537461726b6e65742e6964", "0xb",
			},
			want:       "Starknet.id",
			wantOffset: 3,
		}, {
			name: "test 2",
			response: []data.Felt{
				"0x1", "0x68747470733a2f2f6170692e737461726b6e65742e69642f7572693f69643d", "0x313937393433393730333538", "0xc",
			},
			want:       "https://api.starknet.id/uri?id=197943970358",
			wantOffset: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, offset, err := parseStringArray(tt.response)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantOffset, offset)
		})
	}
}
