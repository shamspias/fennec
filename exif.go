package fennec

import (
	"encoding/binary"
	"image"
	"io"
)

// Orientation describes an EXIF orientation tag value.
type Orientation int

const (
	OrientNormal      Orientation = 1
	OrientFlipH       Orientation = 2
	OrientRotate180   Orientation = 3
	OrientFlipV       Orientation = 4
	OrientTranspose   Orientation = 5 // Rotate 270 CW + flip H
	OrientRotate90CW  Orientation = 6
	OrientTransverse  Orientation = 7 // Rotate 90 CW + flip H
	OrientRotate270CW Orientation = 8
)

// ReadOrientation reads the EXIF orientation tag from a JPEG stream.
// Returns OrientNormal (1) if no orientation is found or the file is not JPEG.
// This is a minimal parser that only reads the orientation tag — it does not
// parse the full EXIF tree, keeping the zero-dependency promise.
func ReadOrientation(r io.ReadSeeker) Orientation {
	// Read JPEG SOI marker.
	var soi [2]byte
	if _, err := io.ReadFull(r, soi[:]); err != nil {
		return OrientNormal
	}
	if soi[0] != 0xFF || soi[1] != 0xD8 {
		return OrientNormal // Not a JPEG.
	}

	// Scan for APP1 marker (0xFFE1) which contains EXIF data.
	for {
		var marker [2]byte
		if _, err := io.ReadFull(r, marker[:]); err != nil {
			return OrientNormal
		}
		if marker[0] != 0xFF {
			return OrientNormal
		}

		// Skip padding bytes.
		for marker[1] == 0xFF {
			if _, err := io.ReadFull(r, marker[1:]); err != nil {
				return OrientNormal
			}
		}

		// Read segment length.
		var lenBuf [2]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return OrientNormal
		}
		segLen := int(binary.BigEndian.Uint16(lenBuf[:])) - 2

		if segLen < 0 {
			return OrientNormal
		}

		if marker[1] == 0xE1 { // APP1
			return parseAPP1(r, segLen)
		}

		// Skip SOS marker — no more metadata after this.
		if marker[1] == 0xDA {
			return OrientNormal
		}

		// Skip this segment.
		if _, err := r.Seek(int64(segLen), io.SeekCurrent); err != nil {
			return OrientNormal
		}
	}
}

// parseAPP1 parses an APP1 segment for EXIF orientation.
func parseAPP1(r io.ReadSeeker, segLen int) Orientation {
	if segLen < 14 {
		return OrientNormal
	}

	// Read the segment data.
	data := make([]byte, segLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return OrientNormal
	}

	// Check for "Exif\0\0" header.
	if len(data) < 6 || string(data[:4]) != "Exif" || data[4] != 0 || data[5] != 0 {
		return OrientNormal
	}

	tiff := data[6:]
	if len(tiff) < 8 {
		return OrientNormal
	}

	// Determine byte order from TIFF header.
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return OrientNormal
	}

	// Verify TIFF magic number (42).
	if bo.Uint16(tiff[2:4]) != 42 {
		return OrientNormal
	}

	// Get offset to first IFD.
	ifdOffset := int(bo.Uint32(tiff[4:8]))
	if ifdOffset < 8 || ifdOffset+2 > len(tiff) {
		return OrientNormal
	}

	// Read IFD0 entry count.
	entryCount := int(bo.Uint16(tiff[ifdOffset : ifdOffset+2]))
	ifdOffset += 2

	// Scan IFD entries for orientation tag (0x0112).
	for i := 0; i < entryCount; i++ {
		entryOff := ifdOffset + i*12
		if entryOff+12 > len(tiff) {
			break
		}

		tag := bo.Uint16(tiff[entryOff : entryOff+2])
		if tag == 0x0112 { // Orientation tag
			dataType := bo.Uint16(tiff[entryOff+2 : entryOff+4])
			if dataType != 3 { // SHORT type
				return OrientNormal
			}
			val := bo.Uint16(tiff[entryOff+8 : entryOff+10])
			if val >= 1 && val <= 8 {
				return Orientation(val)
			}
			return OrientNormal
		}
	}

	return OrientNormal
}

// ApplyOrientation applies EXIF orientation to an NRGBA image,
// producing a correctly-oriented image with orientation = 1.
func ApplyOrientation(img *image.NRGBA, orient Orientation) *image.NRGBA {
	switch orient {
	case OrientNormal, 0:
		return img
	case OrientFlipH:
		return flipNRGBAHorizontal(img)
	case OrientRotate180:
		return rotateNRGBA180(img)
	case OrientFlipV:
		return flipNRGBAVertical(img)
	case OrientTranspose:
		// Rotate 270 CW, then flip horizontal.
		rotated := rotateNRGBA270CW(img)
		return flipNRGBAHorizontal(rotated)
	case OrientRotate90CW:
		return rotateNRGBA90CW(img)
	case OrientTransverse:
		// Rotate 90 CW, then flip horizontal.
		rotated := rotateNRGBA90CW(img)
		return flipNRGBAHorizontal(rotated)
	case OrientRotate270CW:
		return rotateNRGBA270CW(img)
	default:
		return img
	}
}
