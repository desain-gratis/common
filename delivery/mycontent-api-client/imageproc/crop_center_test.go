package imageproc

import (
	"image"
	"reflect"
	"testing"
)

func TestCropByCenterAndscale(t *testing.T) {
	type args struct {
		rect   image.Rectangle
		center image.Point
		ratioX int
		ratioY int
		scale  int
	}
	tests := []struct {
		name string
		args args
		want image.Rectangle
	}{
		{
			name: "Unit test case",
			args: args{
				rect:   image.Rect(0, 0, 100, 100),
				center: image.Pt(50, 50),
				ratioX: 100,
				ratioY: 100,
				scale:  1,
			},
			want: image.Rect(0, 0, 100, 100),
		},
		{
			name: "Centered. 1x1. 1400 x 1970.",
			args: args{
				rect:   image.Rect(0, 0, 1400, 1970),
				center: image.Pt(1400/2, 1970/2),
				ratioX: 1,
				ratioY: 1,
				scale:  1,
			},
			want: image.Rect(0, 285, 1400, 1685),
		},
		{
			name: "Centered. 1x2. 1400 x 1970",
			args: args{
				rect:   image.Rect(0, 0, 1400, 1970),
				center: image.Pt(1400/2, 1970/2),
				ratioX: 1,
				ratioY: 2,
				scale:  1,
			},
			want: image.Rect(207, 0, 1192, 1970),
		},
		{
			name: "Centered. 1400x1970. 1400x1970. Should return the same rectangle.",
			args: args{
				rect:   image.Rect(0, 0, 1400, 1970),
				center: image.Pt(1400/2, 1970/2),
				ratioX: 1400,
				ratioY: 1970,
				scale:  1,
			},
			want: image.Rect(0, 0, 1400, 1970),
		},
		{
			name: "Centered. 1x1. 300x400.",
			args: args{
				rect:   image.Rect(0, 0, 300, 400),
				center: image.Pt(150, 200),
				ratioX: 1,
				ratioY: 1,
				scale:  1,
			},
			want: image.Rect(0, 50, 300, 350),
		},
		{
			name: "Centered. 16x9. 1600x900. Scaled 0.5",
			args: args{
				rect:   image.Rect(0, 0, 1600, 900),
				center: image.Pt(800, 450),
				ratioX: 16,
				ratioY: 9,
				scale:  1,
			},
			want: image.Rect(400, 225, 1200, 675),
		},
		{
			name: "Centered. 4x3. 1920x1080. Scaled 1",
			args: args{
				rect:   image.Rect(0, 0, 1920, 1080),
				center: image.Pt(960, 540),
				ratioX: 4,
				ratioY: 3,
				scale:  1,
			},
			want: image.Rect(240, 0, 1680, 1080),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cropByCenterAndScale(tt.args.rect, tt.args.center.X, tt.args.center.Y, tt.args.ratioX, tt.args.ratioY, tt.args.scale); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CropByCenterAndScale() = %v, want %v", got, tt.want)
			}
		})
	}
}
