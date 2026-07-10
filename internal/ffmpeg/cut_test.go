package ffmpeg

import "testing"

func TestValidateCutOptionsInterpretFPS(t *testing.T) {
	tests := []struct {
		name    string
		options CutOptions
		wantErr bool
	}{
		{
			name:    "disabled does not require fps",
			options: CutOptions{},
			wantErr: false,
		},
		{
			name: "valid slow motion interpret fps",
			options: CutOptions{
				InterpretFPSEnabled: true,
				SourceFPS:           100,
				PlaybackFPS:         25,
			},
			wantErr: false,
		},
		{
			name: "source fps is required",
			options: CutOptions{
				InterpretFPSEnabled: true,
				PlaybackFPS:         25,
			},
			wantErr: true,
		},
		{
			name: "playback fps cannot be lower than 25",
			options: CutOptions{
				InterpretFPSEnabled: true,
				SourceFPS:           100,
				PlaybackFPS:         24,
			},
			wantErr: true,
		},
		{
			name: "playback fps must be lower than source fps",
			options: CutOptions{
				InterpretFPSEnabled: true,
				SourceFPS:           25,
				PlaybackFPS:         25,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCutOptions(tt.options)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCutOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
