package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/cshum/vipsgen/vips"
)

var (
	vipsInitOnce    sync.Once
	vipsInitialized bool
)

func initVips() {
	vipsInitOnce.Do(func() {
		vips.Startup(&vips.Config{
			ConcurrencyLevel: 1,
			MaxCacheFiles:    0,
			MaxCacheMem:      0,
			MaxCacheSize:     0,
		})
		vipsInitialized = true
	})
}

type ResizeParams struct {
	Width       int
	Height      int
	KeepAspect  bool
	Percentage  bool
	AutoCrop    bool // centered cropping
	Crop        bool
	CropOffsetX int
	CropOffsetY int
}

func parseResizeString(resize string) (*ResizeParams, error) {
	params := &ResizeParams{
		KeepAspect: true,
	}

	// Handle percentage resize
	if strings.Contains(resize, "%") {
		params.Percentage = true
		resize = strings.ReplaceAll(resize, "%", "")
	}

	// Handle crop with offset (support both + and _ as separators)
	var cropSeparator string
	if strings.Contains(resize, "+") {
		cropSeparator = "+"
	} else if strings.Contains(resize, "_") {
		cropSeparator = "_"
	}

	if cropSeparator != "" {
		parts := strings.Split(resize, cropSeparator)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid crop offset format")
		}
		resize = parts[0]
		x, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid crop offset X")
		}
		y, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid crop offset Y")
		}
		params.CropOffsetX = x
		params.CropOffsetY = y
		params.Crop = true
	}

	// Handle forced aspect ratio
	if strings.HasSuffix(resize, "!") {
		params.KeepAspect = false
		resize = strings.TrimSuffix(resize, "!")
	}

	// Handle center crop (support both # and ^ as suffixes)
	if strings.HasSuffix(resize, "#") {
		params.AutoCrop = true
		resize = strings.TrimSuffix(resize, "#")
	} else if strings.HasSuffix(resize, "^") {
		params.AutoCrop = true
		resize = strings.TrimSuffix(resize, "^")
	}

	// parse remaining dimensions
	dimensions := strings.Split(resize, "x")
	if len(dimensions) != 2 {
		return nil, fmt.Errorf("invalid resize format")
	}

	// Parse width
	if dimensions[0] != "" {
		width, err := strconv.Atoi(dimensions[0])
		if err != nil {
			return nil, fmt.Errorf("invalid width")
		}
		params.Width = width
	}

	// Parse height
	if dimensions[1] != "" {
		height, err := strconv.Atoi(dimensions[1])
		if err != nil {
			return nil, fmt.Errorf("invalid height")
		}
		params.Height = height
	}

	return params, nil
}

func getImageFormat(image *vips.Image) (vips.ImageType, error) {
	if image == nil {
		return vips.ImageTypeUnknown, fmt.Errorf("image is nil")
	}
	format := image.Format()
	if format == vips.ImageTypeUnknown {
		return format, fmt.Errorf("unknown image format")
	}
	return format, nil
}

func getContentType(imageType vips.ImageType) string {
	switch imageType {
	case vips.ImageTypeJpeg:
		return "image/jpeg"
	case vips.ImageTypePng:
		return "image/png"
	case vips.ImageTypeWebp:
		return "image/webp"
	case vips.ImageTypeGif:
		return "image/gif"
	case vips.ImageTypeTiff:
		return "image/tiff"
	default:
		return "application/octet-stream"
	}
}

func exportImage(image *vips.Image, format vips.ImageType) ([]byte, error) {
	switch format {
	case vips.ImageTypeJpeg:
		opts := vips.DefaultJpegsaveBufferOptions()
		opts.Q = 95
		return image.JpegsaveBuffer(opts)
	case vips.ImageTypePng:
		opts := vips.DefaultPngsaveBufferOptions()
		opts.Compression = 6
		return image.PngsaveBuffer(opts)
	case vips.ImageTypeWebp:
		opts := vips.DefaultWebpsaveBufferOptions()
		opts.Q = 90
		return image.WebpsaveBuffer(opts)
	case vips.ImageTypeGif:
		return image.GifsaveBuffer(nil)
	case vips.ImageTypeTiff:
		return image.TiffsaveBuffer(nil)
	default:
		return nil, fmt.Errorf("unsupported image format")
	}
}

func resizeImage(buf []byte, params *ResizeParams) ([]byte, vips.ImageType, error) {
	// Initialize vips if not already done
	initVips()

	image, err := vips.NewImageFromBuffer(buf, nil)
	if err != nil {
		return nil, vips.ImageTypeUnknown, err
	}
	defer image.Close()

	format, err := getImageFormat(image)
	if err != nil {
		return nil, vips.ImageTypeUnknown, err
	}

	// manual cropping, no scaling takes place
	if params.Crop {
		err = image.ExtractArea(params.CropOffsetX, params.CropOffsetY, params.Width, params.Height)
		if err != nil {
			return nil, format, err
		}
	} else {
		// source dimensions
		width := image.Width()
		height := image.Height()

		// dest dimensions
		targetWidth := params.Width
		targetHeight := params.Height

		// multiply percentages
		if params.Percentage {
			targetWidth = width * params.Width / 100
			targetHeight = height * params.Height / 100
		}

		if params.KeepAspect {
			scaleRatio := 1.0
			if targetWidth == 0 {
				scaleRatio = float64(targetHeight) / float64(height)
			} else if targetHeight == 0 {
				scaleRatio = float64(targetWidth) / float64(width)
			} else {
				// Scale proportionally to fit within target dimensions
				widthRatio := float64(targetWidth) / float64(width)
				heightRatio := float64(targetHeight) / float64(height)

				if params.AutoCrop {
					scaleRatio = math.Max(widthRatio, heightRatio)
				} else {
					scaleRatio = math.Min(widthRatio, heightRatio)
				}
			}
			if scaleRatio <= 0 {
				return nil, format, fmt.Errorf("invalid scale ratio")
			}
			if err := image.Resize(scaleRatio, nil); err != nil {
				return nil, format, err
			}
		} else {
			widthScale := float64(targetWidth) / float64(width)
			heightScale := float64(targetHeight) / float64(height)
			if widthScale <= 0 || heightScale <= 0 {
				return nil, format, fmt.Errorf("invalid scale ratio")
			}
			opts := vips.DefaultResizeOptions()
			opts.Vscale = heightScale
			if err := image.Resize(widthScale, opts); err != nil {
				return nil, format, err
			}
		}

		if params.AutoCrop {
			// Center crop
			left := (image.Width() - params.Width) / 2
			top := (image.Height() - params.Height) / 2
			err = image.ExtractArea(left, top, params.Width, params.Height)
			if err != nil {
				return nil, format, err
			}
		}
	}

	resized, err := exportImage(image, format)
	return resized, format, err
}
