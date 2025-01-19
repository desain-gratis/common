package imageproc

// Import the package to access the Wasm environment
import (
	"image"
	"image/draw"

	draw2 "golang.org/x/image/draw"
)

// https://github.com/desain-gratis/image-upload-wasm/blob/main/main.go
func Scale(img image.Image, axis string, target int) image.Image {
	scale := float64(target) / float64(img.Bounds().Dx())
	newWidth := target
	newHeight := int(float64(img.Bounds().Dy()) * scale)

	if axis == "height" {
		scale = float64(target) / float64(img.Bounds().Dy())
		newWidth = int(float64(img.Bounds().Dx()) * scale)
		newHeight = target
	}

	scaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw2.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Over, nil)
	return scaled
}

// https://github.com/desain-gratis/image-upload-wasm/blob/main/main.go
// img is the image that want to be cropped and scaled
// offsetX and offsetY from center for the cropped image
// ratioX and ratioY is the image ratio
// by default the resulting image is the maximum area allowed by the original image and given ratio
// to prevent image too large, use maximumBoundingSquareWidth
func Crop(img image.Image, offsetX int, offsetY int, ratioX int, ratioY int) *image.RGBA {
	if ratioX <= 0 {
		ratioX = img.Bounds().Dx()
	}
	if ratioY <= 0 {
		ratioY = img.Bounds().Dy()
	}

	crop := cropByCenterAndScale(img.Bounds(), img.Bounds().Dx()/2+offsetX, img.Bounds().Dy()/2+offsetY, ratioX, ratioY, 0)

	// todo can use sub image
	cropped := image.NewRGBA(image.Rect(0, 0, crop.Max.X-crop.Min.X, crop.Max.Y-crop.Min.Y))
	draw.Draw(cropped, cropped.Rect, img, crop.Min, draw.Src)

	return cropped
}

// cropByCenterAndScale
// @dprecated param boundingSquareWidth
func cropByCenterAndScale(rect image.Rectangle, centerX int, centerY int, ratioX int, ratioY int, boundingSquareWidth int) image.Rectangle {
	// We just need to find the max width here or use spoke
	x1 := centerX
	x2 := rect.Max.X - centerX
	y1 := centerY
	y2 := rect.Max.Y - centerY

	// treat target as ratio
	_ratioX := float64(ratioX)
	_ratioY := float64(ratioY)

	minX := x1
	if x2 < x1 {
		minX = x2
	}
	minY := y1
	if y2 < y1 {
		minY = y2
	}

	// how many height can be taken, given limited width (min width)
	// how many width can be taken, given limited height (min height)
	maxWidth := float64(minY) * _ratioX / _ratioY
	maxHeight := float64(minX) * _ratioY / _ratioX

	var width, height float64

	width = float64(minX)
	if float64(minX) > maxWidth {
		width = maxWidth
	}
	height = float64(minY)
	if float64(minY) > maxHeight {
		height = maxHeight
	}

	var boundedByWidth bool
	if width > height {
		boundedByWidth = true
	}

	if boundedByWidth {
		height = float64(width) * _ratioY / _ratioX
	} else {
		width = float64(height) * _ratioX / _ratioY
	}

	// The "actual" maximum bounded by the image
	width = width * 2
	height = height * 2

	// scale to fit the bounding square width (if specified)
	// this is to scale the "cropper",  not actualy scale the image
	scale := float64(1)
	if boundedByWidth {
		if width > float64(boundingSquareWidth) && boundingSquareWidth > 0 {
			scale = float64(boundingSquareWidth) / width
		}
	} else {
		if height > float64(boundingSquareWidth) && boundingSquareWidth > 0 {
			scale = float64(boundingSquareWidth) / height
		}
	}
	width = float64(width) * scale
	height = float64(height) * scale

	// if float64(targetWidth) < width {
	// 	width = float64(targetWidth)
	// }
	// if float64(targetHeight) < height {
	// 	height = float64(targetHeight)
	// }

	return cropByCenterWidthHeight(centerX, centerY, int(width), int(height))
}

func cropByCenterWidthHeight(centerX int, centerY int, width int, height int) image.Rectangle {
	_minx := float64(centerX) - float64(width)/2
	_miny := float64(centerY) - float64(height)/2
	_maxx := float64(centerX) + float64(width)/2
	_maxy := float64(centerY) + float64(height)/2

	return image.Rect(int(_minx), int(_miny), int(_maxx), int(_maxy))
}
