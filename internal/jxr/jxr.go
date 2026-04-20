package jxr

import (
	"encoding/binary"
	"fmt"
)

const (
	ColorYOnly = 0

	BitDepth8 = 1

	BandsAll = 0
)

type Container struct {
	PixelFormat string
	Width       int
	Height      int
	ImageData   []byte
}

type Header struct {
	HardTiling       int
	Tiling           int
	FrequencyMode    int
	Orientation      int
	IndexTable       int
	OverlapMode      int
	ShortHeader      int
	LongWord         int
	Windowing        int
	TrimFlexbits     int
	RedBlueNotSwapped int
	Alpha            int
	OutputColor      int
	OutputBitDepth   int
	ImageWidth       int
	ImageHeight      int
	ExtraTop         int
	ExtraLeft        int
	ExtraBottom      int
	ExtraRight       int
	MBWidth          int
	MBHeight         int
	Plane            PlaneHeader
}

type PlaneHeader struct {
	InternalColor    int
	Scaled           int
	BandsPresent     int
	NumBands         int
	NumComponents    int
	DCUniform        int
	LPUniform        int
	HPUniform        int
}

func (h Header) SupportsFixtureGraySubset() bool {
	return h.HardTiling == 0 &&
		h.Tiling == 0 &&
		h.FrequencyMode == 0 &&
		h.IndexTable == 0 &&
		h.OverlapMode == 0 &&
		h.ShortHeader == 1 &&
		h.LongWord == 1 &&
		h.Windowing == 0 &&
		h.TrimFlexbits == 0 &&
		h.Alpha == 0 &&
		h.OutputColor == ColorYOnly &&
		h.OutputBitDepth == BitDepth8 &&
		h.Plane.InternalColor == ColorYOnly &&
		h.Plane.Scaled <= 1 &&
		h.Plane.BandsPresent == BandsAll &&
		h.Plane.NumBands == 4 &&
		h.Plane.NumComponents == 1 &&
		h.Plane.DCUniform == 1 &&
		h.Plane.LPUniform == 1 &&
		h.Plane.HPUniform == 1
}

func ParseContainer(data []byte) (Container, error) {
	var c Container
	if len(data) < 8 {
		return c, fmt.Errorf("jxr container is too short")
	}
	if string(data[:4]) != "\x49\x49\xbc\x01" {
		return c, fmt.Errorf("invalid JXR container signature")
	}

	ifdOffset := int(binary.LittleEndian.Uint32(data[4:8]))
	if ifdOffset < 8 || ifdOffset+2 > len(data) {
		return c, fmt.Errorf("invalid JXR IFD offset")
	}
	cursor := ifdOffset
	entryCount := int(binary.LittleEndian.Uint16(data[cursor : cursor+2]))
	cursor += 2

	var imageOffset, imageByteCount int
	for i := 0; i < entryCount; i++ {
		if cursor+12 > len(data) {
			return c, fmt.Errorf("truncated JXR IFD entry")
		}
		tag := binary.LittleEndian.Uint16(data[cursor : cursor+2])
		fieldType := binary.LittleEndian.Uint16(data[cursor+2 : cursor+4])
		count := binary.LittleEndian.Uint32(data[cursor+4 : cursor+8])
		fieldLen, ok := fieldTypeLen[fieldType]
		if !ok {
			return c, fmt.Errorf("unsupported JXR TIFF field type %d", fieldType)
		}
		dataLen := int(count) * fieldLen
		rawField := data[cursor+8 : cursor+12]
		var fieldData []byte
		if dataLen <= 4 {
			fieldData = rawField[:dataLen]
		} else {
			if cursor+12 > len(data) {
				return c, fmt.Errorf("truncated JXR field pointer")
			}
			offset := int(binary.LittleEndian.Uint32(rawField))
			if offset < 0 || offset+dataLen > len(data) {
				return c, fmt.Errorf("JXR field data is out of range")
			}
			fieldData = data[offset : offset+dataLen]
		}

		switch tag {
		case 0xbc01:
			if len(fieldData) != 16 {
				return c, fmt.Errorf("unexpected JXR pixel format size %d", len(fieldData))
			}
			c.PixelFormat = formatUUID(fieldData)
		case 0xbc80:
			value, err := fieldValue(fieldType, fieldData)
			if err != nil {
				return c, err
			}
			c.Width = value
		case 0xbc81:
			value, err := fieldValue(fieldType, fieldData)
			if err != nil {
				return c, err
			}
			c.Height = value
		case 0xbcc0:
			value, err := fieldValue(fieldType, fieldData)
			if err != nil {
				return c, err
			}
			imageOffset = value
		case 0xbcc1:
			value, err := fieldValue(fieldType, fieldData)
			if err != nil {
				return c, err
			}
			imageByteCount = value
		}

		cursor += 12
	}

	if c.PixelFormat == "" || c.Width == 0 || c.Height == 0 || imageOffset == 0 {
		return c, fmt.Errorf("missing required JXR container fields")
	}
	if imageOffset < 0 || imageOffset > len(data) {
		return c, fmt.Errorf("invalid JXR image data offset")
	}
	if imageByteCount > 0 {
		if imageOffset+imageByteCount > len(data) {
			return c, fmt.Errorf("truncated JXR image data")
		}
		c.ImageData = append([]byte(nil), data[imageOffset:imageOffset+imageByteCount]...)
	} else {
		c.ImageData = append([]byte(nil), data[imageOffset:]...)
	}
	return c, nil
}

func ParseHeader(data []byte) (Header, error) {
	var h Header
	br := newBitReader(data)

	signature, err := br.readBytes(8)
	if err != nil {
		return h, err
	}
	if string(signature) != "WMPHOTO\x00" {
		return h, fmt.Errorf("invalid JXR image signature")
	}

	if version, err := br.readBits(4); err != nil || version != 1 {
		if err != nil {
			return h, err
		}
		return h, fmt.Errorf("unsupported JXR codec version %d", version)
	}
	if h.HardTiling, err = br.readBits(1); err != nil {
		return h, err
	}
	if subversion, err := br.readBits(3); err != nil || subversion != 1 {
		if err != nil {
			return h, err
		}
		return h, fmt.Errorf("unsupported JXR codec subversion %d", subversion)
	}
	if h.Tiling, err = br.readBits(1); err != nil {
		return h, err
	}
	if h.FrequencyMode, err = br.readBits(1); err != nil {
		return h, err
	}
	if h.Orientation, err = br.readBits(3); err != nil {
		return h, err
	}
	if h.IndexTable, err = br.readBits(1); err != nil {
		return h, err
	}
	if h.OverlapMode, err = br.readBits(2); err != nil {
		return h, err
	}
	if h.ShortHeader, err = br.readBits(1); err != nil {
		return h, err
	}
	if h.LongWord, err = br.readBits(1); err != nil {
		return h, err
	}
	if h.Windowing, err = br.readBits(1); err != nil {
		return h, err
	}
	if h.TrimFlexbits, err = br.readBits(1); err != nil {
		return h, err
	}
	if reserved, err := br.readBits(1); err != nil || reserved != 0 {
		if err != nil {
			return h, err
		}
		return h, fmt.Errorf("unexpected reserved bit %d", reserved)
	}
	if h.RedBlueNotSwapped, err = br.readBits(1); err != nil {
		return h, err
	}
	if premultiplied, err := br.readBits(1); err != nil || premultiplied != 0 {
		if err != nil {
			return h, err
		}
		return h, fmt.Errorf("unsupported premultiplied alpha flag %d", premultiplied)
	}
	if h.Alpha, err = br.readBits(1); err != nil {
		return h, err
	}
	if h.OutputColor, err = br.readBits(4); err != nil {
		return h, err
	}
	if h.OutputBitDepth, err = br.readBits(4); err != nil {
		return h, err
	}

	if h.ShortHeader == 1 {
		if h.ImageWidth, err = br.readUint16BEPlusOne(); err != nil {
			return h, err
		}
		if h.ImageHeight, err = br.readUint16BEPlusOne(); err != nil {
			return h, err
		}
	} else {
		return h, fmt.Errorf("long JXR headers are not supported")
	}

	if h.Tiling == 1 {
		return h, fmt.Errorf("tiled JXR images are not supported")
	}

	if h.Windowing == 1 {
		if h.ExtraTop, err = br.readBits(6); err != nil {
			return h, err
		}
		if h.ExtraLeft, err = br.readBits(6); err != nil {
			return h, err
		}
		if h.ExtraBottom, err = br.readBits(6); err != nil {
			return h, err
		}
		if h.ExtraRight, err = br.readBits(6); err != nil {
			return h, err
		}
	} else {
		h.ExtraRight = extraPadding(h.ImageWidth)
		h.ExtraBottom = extraPadding(h.ImageHeight)
	}

	width := h.ImageWidth + h.ExtraLeft + h.ExtraRight
	height := h.ImageHeight + h.ExtraTop + h.ExtraBottom
	h.MBWidth = width / 16
	h.MBHeight = height / 16

	plane, err := parsePlaneHeader(br, h.OutputBitDepth)
	if err != nil {
		return h, err
	}
	h.Plane = plane
	return h, nil
}

var fieldTypeLen = map[uint16]int{
	1:  1,
	2:  1,
	3:  2,
	4:  4,
	5:  8,
	6:  1,
	7:  1,
	8:  2,
	9:  4,
	10: 8,
	11: 4,
	12: 8,
}

type bitReader struct {
	data          []byte
	byteOffset    int
	bitsRemaining int
	remainder     uint64
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data}
}

func (r *bitReader) readBytes(size int) ([]byte, error) {
	if r.bitsRemaining != 0 {
		return nil, fmt.Errorf("unexpected %d remaining bits", r.bitsRemaining)
	}
	if size < 0 || r.byteOffset+size > len(r.data) {
		return nil, fmt.Errorf("insufficient data")
	}
	out := r.data[r.byteOffset : r.byteOffset+size]
	r.byteOffset += size
	return out, nil
}

func (r *bitReader) readBits(size int) (int, error) {
	if size < 0 {
		return 0, fmt.Errorf("invalid bit count %d", size)
	}
	for r.bitsRemaining < size {
		if r.byteOffset >= len(r.data) {
			return 0, fmt.Errorf("insufficient bitstream data")
		}
		r.remainder = (r.remainder << 8) | uint64(r.data[r.byteOffset])
		r.byteOffset++
		r.bitsRemaining += 8
	}
	r.bitsRemaining -= size
	value := int(r.remainder >> r.bitsRemaining)
	if r.bitsRemaining == 0 {
		r.remainder = 0
	} else {
		r.remainder &= (uint64(1) << r.bitsRemaining) - 1
	}
	return value, nil
}

func (r *bitReader) readUint16BEPlusOne() (int, error) {
	hi, err := r.readBits(8)
	if err != nil {
		return 0, err
	}
	lo, err := r.readBits(8)
	if err != nil {
		return 0, err
	}
	return (hi<<8 | lo) + 1, nil
}

func fieldValue(fieldType uint16, data []byte) (int, error) {
	switch fieldType {
	case 1, 6:
		if len(data) < 1 {
			return 0, fmt.Errorf("truncated field value")
		}
		return int(data[0]), nil
	case 3, 8:
		if len(data) < 2 {
			return 0, fmt.Errorf("truncated field value")
		}
		return int(binary.LittleEndian.Uint16(data[:2])), nil
	case 4, 9:
		if len(data) < 4 {
			return 0, fmt.Errorf("truncated field value")
		}
		return int(binary.LittleEndian.Uint32(data[:4])), nil
	default:
		return 0, fmt.Errorf("unsupported field value type %d", fieldType)
	}
}

func formatUUID(data []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(data[0:4]),
		binary.BigEndian.Uint16(data[4:6]),
		binary.BigEndian.Uint16(data[6:8]),
		binary.BigEndian.Uint16(data[8:10]),
		uint64(data[10])<<40|uint64(data[11])<<32|uint64(data[12])<<24|uint64(data[13])<<16|uint64(data[14])<<8|uint64(data[15]),
	)
}

func extraPadding(value int) int {
	if rem := value & 0xF; rem != 0 {
		return 0x10 - rem
	}
	return 0
}

func parsePlaneHeader(br *bitReader, outputBitDepth int) (PlaneHeader, error) {
	var p PlaneHeader
	var err error
	if p.InternalColor, err = br.readBits(3); err != nil {
		return p, err
	}
	if p.Scaled, err = br.readBits(1); err != nil {
		return p, err
	}
	if p.BandsPresent, err = br.readBits(4); err != nil {
		return p, err
	}

	p.NumBands = 1
	if p.BandsPresent != 3 {
		p.NumBands++
		if p.BandsPresent != 2 {
			p.NumBands++
			if p.BandsPresent != 1 {
				p.NumBands++
			}
		}
	}

	switch p.InternalColor {
	case ColorYOnly:
		p.NumComponents = 1
	default:
		return p, fmt.Errorf("unsupported JXR internal color format %d", p.InternalColor)
	}

	if outputBitDepth == 2 || outputBitDepth == 3 || outputBitDepth == 6 {
		if _, err := br.readBits(8); err != nil {
			return p, err
		}
	}
	if outputBitDepth == 7 {
		if _, err := br.readBits(8); err != nil {
			return p, err
		}
		if _, err := br.readBits(8); err != nil {
			return p, err
		}
	}

	if p.DCUniform, err = br.readBits(1); err != nil {
		return p, err
	}
	if p.DCUniform == 1 {
		if err := skipQP(br, p.NumComponents, p.Scaled, 0); err != nil {
			return p, err
		}
	}

	p.LPUniform = -1
	p.HPUniform = -1
	if p.BandsPresent != 3 {
		if reserved, err := br.readBits(1); err != nil || reserved != 0 {
			if err != nil {
				return p, err
			}
			return p, fmt.Errorf("unexpected JXR reserved plane bit %d", reserved)
		}
		if p.LPUniform, err = br.readBits(1); err != nil {
			return p, err
		}
		if p.LPUniform == 1 {
			if err := skipQP(br, p.NumComponents, p.Scaled, 1); err != nil {
				return p, err
			}
		}
		if p.BandsPresent != 2 {
			if reserved, err := br.readBits(1); err != nil || reserved != 0 {
				if err != nil {
					return p, err
				}
				return p, fmt.Errorf("unexpected JXR reserved HP plane bit %d", reserved)
			}
			if p.HPUniform, err = br.readBits(1); err != nil {
				return p, err
			}
			if p.HPUniform == 1 {
				if err := skipQP(br, p.NumComponents, p.Scaled, 2); err != nil {
					return p, err
				}
			}
		}
	}

	return p, nil
}

func skipQP(br *bitReader, numComponents int, scaled int, band int) error {
	// Port of the narrow QP shape used by the grayscale Martyr fixtures:
	// one component, single QP, so Python's QP parser always selects the
	// implicit UNIFORM component mode and consumes exactly one 8-bit value.
	if numComponents != 1 {
		return fmt.Errorf("unsupported JXR QP component count %d", numComponents)
	}
	if _, err := br.readBits(8); err != nil {
		return err
	}
	return nil
}
