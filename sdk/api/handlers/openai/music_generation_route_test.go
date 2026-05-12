package openai

import "testing"

func TestMusicGenerationRouteModel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"documented text model", "minimax:music-2.6", "ail-music"},
		{"bare text model", "music-2.6", "ail-music"},
		{"prefixed alias text model", "minimax/ail-music", "ail-music"},
		{"documented cover model", "minimax:music-cover", "ail-music-cover"},
		{"bare cover model", "music-cover", "ail-music-cover"},
		{"prefixed alias cover model", "minimax/ail-music-cover", "ail-music-cover"},
		{"unrelated model unchanged", "custom-provider-model", "custom-provider-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := musicGenerationRouteModel(tt.in); got != tt.want {
				t.Fatalf("musicGenerationRouteModel(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
