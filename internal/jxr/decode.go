package jxr

import (
	"fmt"
	"image"
	"math"
)

const (
	bandDC = 0
	bandLP = 1
	bandHP = 2

	predictFromLeft    = 0
	predictFromTop     = 1
	predictFromTopLeft = 2
	noPrediction       = 3
)

var (
	ict4x4InvPermArr = [16]int{0, 8, 4, 13, 2, 15, 3, 14, 1, 12, 5, 9, 7, 11, 6, 10}
	iHierScanOrder   = [16]int{0, 4, 1, 5, 8, 12, 9, 13, 2, 6, 3, 7, 10, 14, 11, 15}
	lowpassScanOrder = [16]int{0, 1, 4, 5, 2, 8, 6, 9, 3, 12, 10, 7, 13, 11, 14, 15}
	highpassHorOrder = [16]int{0, 5, 10, 12, 1, 2, 8, 4, 6, 9, 3, 14, 13, 7, 11, 15}
	highpassVerOrder = [16]int{0, 10, 2, 12, 5, 9, 4, 8, 1, 13, 6, 15, 14, 3, 11, 7}
	transposeFlex    = [16]int{0, 5, 1, 6, 10, 12, 8, 14, 2, 4, 3, 7, 9, 13, 11, 15}
	mbPixelMap       = [16]int{0, 1, 5, 4, 2, 3, 7, 6, 10, 11, 15, 14, 8, 9, 13, 12}

	numCBPHP = [2]map[int]int{
		hbin(map[string]int{"1": 0, "01": 1, "001": 2, "0000": 3, "0001": 4}),
		hbin(map[string]int{"1": 0, "000": 1, "001": 2, "010": 3, "011": 4}),
	}
	numBlkCBPHP1 = [2]map[int]int{
		hbin(map[string]int{"1": 0, "01": 1, "001": 2, "0000": 3, "0001": 4}),
		hbin(map[string]int{"1": 0, "000": 1, "001": 2, "010": 3, "011": 4}),
	}
	firstIndex = [5]map[int]int{
		hbin(map[string]int{"00001": 0, "000001": 1, "0000000": 2, "0000001": 3, "00100": 4, "010": 5, "00101": 6, "1": 7, "00110": 8, "0001": 9, "00111": 10, "011": 11}),
		hbin(map[string]int{"0010": 0, "00010": 1, "000000": 2, "000001": 3, "0011": 4, "010": 5, "00011": 6, "11": 7, "011": 8, "100": 9, "00001": 10, "101": 11}),
		hbin(map[string]int{"11": 0, "001": 1, "0000000": 2, "0000001": 3, "00001": 4, "010": 5, "0000010": 6, "011": 7, "100": 8, "101": 9, "0000011": 10, "0001": 11}),
		hbin(map[string]int{"001": 0, "11": 1, "0000000": 2, "00001": 3, "00010": 4, "010": 5, "0000001": 6, "011": 7, "00011": 8, "100": 9, "000001": 10, "101": 11}),
		hbin(map[string]int{"010": 0, "1": 1, "0000001": 2, "0001": 3, "0000010": 4, "011": 5, "00000000": 6, "0010": 7, "0000011": 8, "0011": 9, "00000001": 10, "00001": 11}),
	}
	firstIndexDelta = [4][12]int{
		{1, 1, 1, 1, 1, 0, 0, -1, 2, 1, 0, 0},
		{2, 2, -1, -1, -1, 0, -2, -1, 0, 0, -2, -1},
		{-1, 1, 0, 2, 0, 0, 0, 0, -2, 0, 1, 1},
		{0, 1, 0, 1, -2, 0, -1, -1, -2, -1, -2, -2},
	}
	index1Delta = [3][6]int{
		{-1, 1, 1, 1, 0, 1},
		{-2, 0, 0, 2, 0, 0},
		{-1, -1, 0, 1, -2, 0},
	}
	indexA = [4]map[int]int{
		hbin(map[string]int{"1": 0, "00000": 1, "001": 2, "00001": 3, "01": 4, "0001": 5}),
		hbin(map[string]int{"01": 0, "0000": 1, "10": 2, "0001": 3, "11": 4, "001": 5}),
		hbin(map[string]int{"0000": 0, "0001": 1, "01": 2, "10": 3, "11": 4, "001": 5}),
		hbin(map[string]int{"00000": 0, "00001": 1, "01": 2, "1": 3, "0001": 4, "001": 5}),
	}
	indexB = hbin(map[string]int{"0": 0, "10": 2, "110": 1, "111": 3})
	runIndex = hbin(map[string]int{"1": 0, "01": 1, "001": 2, "0000": 3, "0001": 4})
	runValue = [5]map[int]int{
		nil,
		nil,
		hbin(map[string]int{"1": 1, "0": 2}),
		hbin(map[string]int{"1": 1, "01": 2, "00": 3}),
		hbin(map[string]int{"1": 1, "01": 2, "001": 3, "000": 4}),
	}
	absLevelIndex = [2]map[int]int{
		hbin(map[string]int{"01": 0, "10": 1, "11": 2, "001": 3, "0001": 4, "00000": 5, "00001": 6}),
		hbin(map[string]int{"1": 0, "01": 1, "001": 2, "0001": 3, "00001": 4, "000000": 5, "000001": 6}),
	}
	absLevelIndexDelta = [7]int{1, 0, -1, -1, -1, -1, -1}
	refCBPHP1          = hbin(map[string]int{"00": 3, "01": 5, "100": 6, "101": 9, "110": 10, "111": 12})
	firstModelWeight   = [3]int{240, 12, 1}
)

func DecodeGray8(containerData []byte) (*image.Gray, error) {
	container, err := ParseContainer(containerData)
	if err != nil {
		return nil, err
	}
	decoder := newGrayDecoder(container.ImageData)
	if err := decoder.decode(); err != nil {
		return nil, err
	}
	return decoder.toImage(), nil
}

type grayDecoder struct {
	br           *bitReader
	header       Header
	paddedWidth  int
	paddedHeight int
	plane        *grayPlane
	mbs          [][]*grayMB
	imagePlane   []int
}

type grayPlane struct {
	decoder         *grayDecoder
	dcScale         int
	lpScale         int
	hpScale         int
	flexbitsPresent bool
	dc              dcBand
	lp              lpBand
	hp              hpBand
}

type grayMB struct {
	MBx, MBy, MBxt, MByt                  int
	left, top                             *grayMB
	IsMBLeftEdgeofTileFlag                bool
	IsMBTopEdgeofTileFlag                 bool
	InitializeContext, ResetTotals        bool
	ResetContext                          bool
	MBDCMode, MBLPMode, MBHPMode          int
	HPInputVLC, HPInputFlex               [16][16]int
	MbDCLP                                [16]int
	MBCBPHP                               int
	ModelBitsMBHP, MBQPIndexLP            int
	MBBuffer                              [256]int
}

type grayModel struct {
	MState int
	MBits  int
}

type adaptiveVLC struct {
	TableIndex, DeltaTableIndex, Delta2TableIndex int
	DiscrimVal1, DiscrimVal2                      int
}

type adaptiveScan struct {
	order  [16]int
	totals [16]int
}

type dcBand struct {
	plane       *grayPlane
	model       grayModel
	absLevelLum adaptiveVLC
}

type lpBand struct {
	plane         *grayPlane
	model         grayModel
	firstLum      adaptiveVLC
	indLum0       adaptiveVLC
	indLum1       adaptiveVLC
	absLevel0     adaptiveVLC
	absLevel1     adaptiveVLC
	lowpassScan   adaptiveScan
}

type hpBand struct {
	plane          *grayPlane
	model          grayModel
	numCBPHP       adaptiveVLC
	numBlkCBPHP    adaptiveVLC
	firstLum       adaptiveVLC
	indLum0        adaptiveVLC
	indLum1        adaptiveVLC
	absLevel0      adaptiveVLC
	absLevel1      adaptiveVLC
	highpassHor    adaptiveScan
	highpassVer    adaptiveScan
	cbphpState     int
	countOnes      int
	countZeroes    int
}

func newGrayDecoder(data []byte) *grayDecoder {
	d := &grayDecoder{br: newBitReader(data)}
	plane := &grayPlane{decoder: d}
	plane.dc.plane = plane
	plane.lp.plane = plane
	plane.hp.plane = plane
	d.plane = plane
	return d
}

func (d *grayDecoder) decode() error {
	header, dcScale, lpScale, hpScale, err := d.parseHeaderAndPlane()
	if err != nil {
		return err
	}
	if !header.SupportsFixtureGraySubset() {
		return fmt.Errorf("unsupported JXR fixture profile")
	}
	d.header = header
	d.paddedWidth = header.ImageWidth + header.ExtraLeft + header.ExtraRight
	d.paddedHeight = header.ImageHeight + header.ExtraTop + header.ExtraBottom
	d.plane.dcScale = dcScale
	d.plane.lpScale = lpScale
	d.plane.hpScale = hpScale
	d.plane.flexbitsPresent = true
	d.initMacroblocks()

	subsequentBytes, err := d.vlwEsc()
	if err != nil {
		return err
	}
	if subsequentBytes != 0 {
		if err := d.profileLevelInfo(); err != nil {
			return err
		}
	}
	if err := d.decodeSpatialTile(); err != nil {
		return err
	}
	d.br.discardRemainderBits()

	d.firstLevelInverseTransform()
	d.secondLevelInverseTransform()
	d.secondLevelCoefficientCombination()
	d.outputFormatting()
	return nil
}

func (d *grayDecoder) parseHeaderAndPlane() (Header, int, int, int, error) {
	h, err := ParseHeader(d.br.data)
	if err != nil {
		return Header{}, 0, 0, 0, err
	}

	// Re-read the stream with the live bitreader so decode continues after header parsing.
	br := d.br
	if _, err := br.readBytes(8); err != nil {
		return Header{}, 0, 0, 0, err
	}
	for _, bits := range []int{4, 1, 3, 1, 1, 3, 1, 2, 1, 1, 1, 1, 1, 1, 1, 1, 4, 4} {
		if _, err := br.readBits(bits); err != nil {
			return Header{}, 0, 0, 0, err
		}
	}
	if _, err := br.readBits(16); err != nil {
		return Header{}, 0, 0, 0, err
	}
	if _, err := br.readBits(16); err != nil {
		return Header{}, 0, 0, 0, err
	}
	if h.Windowing == 1 {
		for i := 0; i < 4; i++ {
			if _, err := br.readBits(6); err != nil {
				return Header{}, 0, 0, 0, err
			}
		}
	}

	if _, err := br.readBits(3); err != nil { // internal_clr_fmt
		return Header{}, 0, 0, 0, err
	}
	if _, err := br.readBits(1); err != nil { // scaled_flag
		return Header{}, 0, 0, 0, err
	}
	if _, err := br.readBits(4); err != nil { // bands_present
		return Header{}, 0, 0, 0, err
	}

	dcUniform, err := br.readBits(1)
	if err != nil {
		return Header{}, 0, 0, 0, err
	}
	var dcScale int
	if dcUniform == 1 {
		dcScale, err = readSingleComponentQP(br, true, bandDC)
		if err != nil {
			return Header{}, 0, 0, 0, err
		}
	}

	if _, err := br.readBits(1); err != nil { // reserved_i_bit
		return Header{}, 0, 0, 0, err
	}
	lpUniform, err := br.readBits(1)
	if err != nil {
		return Header{}, 0, 0, 0, err
	}
	var lpScale int
	if lpUniform == 1 {
		lpScale, err = readSingleComponentQP(br, true, bandLP)
		if err != nil {
			return Header{}, 0, 0, 0, err
		}
	}

	if _, err := br.readBits(1); err != nil { // reserved_j_bit
		return Header{}, 0, 0, 0, err
	}
	hpUniform, err := br.readBits(1)
	if err != nil {
		return Header{}, 0, 0, 0, err
	}
	var hpScale int
	if hpUniform == 1 {
		hpScale, err = readSingleComponentQP(br, true, bandHP)
		if err != nil {
			return Header{}, 0, 0, 0, err
		}
	}
	br.discardRemainderBits()
	return h, dcScale, lpScale, hpScale, nil
}

func readSingleComponentQP(br *bitReader, scaled bool, band int) (int, error) {
	quant, err := br.readBits(8)
	if err != nil {
		return 0, err
	}
	return quantMap(quant, scaled, band), nil
}

func quantMap(qp int, scaled bool, band int) int {
	if qp == 0 {
		return 1
	}
	if !scaled {
		notScaledShift := -2
		var man, exp int
		switch {
		case qp < 32:
			man = (qp + 3) >> 2
			exp = 0
		case qp < 48:
			man = (16 + (qp & 15) + 1) >> 1
			exp = (qp >> 4) + notScaledShift
		default:
			man = 16 + (qp & 15)
			exp = ((qp >> 4) - 1) + notScaledShift
		}
		return man << exp
	}

	scaledShift := 1
	if qp < 16 {
		return qp << scaledShift
	}
	man := 16 + (qp & 15)
	exp := ((qp >> 4) - 1) + scaledShift
	return man << exp
}

func (d *grayDecoder) initMacroblocks() {
	d.mbs = make([][]*grayMB, d.header.MBWidth)
	for x := 0; x < d.header.MBWidth; x++ {
		d.mbs[x] = make([]*grayMB, d.header.MBHeight)
	}
	for tx := 0; tx < 1; tx++ {
		firstMBx := 0
		tileMBWidth := d.header.MBWidth
		for ty := 0; ty < 1; ty++ {
			firstMBy := 0
			tileMBHeight := d.header.MBHeight
			for mbxt := 0; mbxt < tileMBWidth; mbxt++ {
				mbx := firstMBx + mbxt
				for mbyt := 0; mbyt < tileMBHeight; mbyt++ {
					mby := firstMBy + mbyt
					var left, top *grayMB
					if mbx > 0 {
						left = d.mbs[mbx-1][mby]
					}
					if mby > 0 {
						top = d.mbs[mbx][mby-1]
					}
					d.mbs[mbx][mby] = &grayMB{
						MBx:                 mbx,
						MBy:                 mby,
						MBxt:                mbxt,
						MByt:                mbyt,
						left:                left,
						top:                 top,
						IsMBLeftEdgeofTileFlag: mbxt == 0,
						IsMBTopEdgeofTileFlag:  mbyt == 0,
						InitializeContext:      mbxt == 0 && mbyt == 0,
						ResetTotals:            (mbxt % 16) == 0,
						ResetContext:           (mbxt % 16) == 0 || mbxt == tileMBWidth-1,
					}
				}
			}
		}
	}
}

func (d *grayDecoder) vlwEsc() (int, error) {
	first, err := d.br.readBits(8)
	if err != nil {
		return 0, err
	}
	switch first {
	case 0xfb:
		return d.br.readBits(32)
	case 0xfc:
		return d.br.readBits(64)
	default:
		if first >= 0xfd {
			return 0, nil
		}
		low, err := d.br.readBits(8)
		if err != nil {
			return 0, err
		}
		return first*256 + low, nil
	}
}

func (d *grayDecoder) profileLevelInfo() error {
	for {
		if _, err := d.br.readBits(8); err != nil {
			return err
		}
		if _, err := d.br.readBits(8); err != nil {
			return err
		}
		if _, err := d.br.readBits(15); err != nil {
			return err
		}
		last, err := d.br.readBits(1)
		if err != nil {
			return err
		}
		if last == 1 {
			return nil
		}
	}
}

func (d *grayDecoder) decodeSpatialTile() error {
	startcode, err := d.br.readBits(24)
	if err != nil {
		return err
	}
	if startcode != 1 {
		return fmt.Errorf("unexpected JXR tile startcode %d", startcode)
	}
	if _, err := d.br.readBits(8); err != nil { // arbitrary byte
		return err
	}

	for mby := 0; mby < d.header.MBHeight; mby++ {
		for mbx := 0; mbx < d.header.MBWidth; mbx++ {
			mb := d.mbs[mbx][mby]
			if err := d.plane.dc.decodeMB(mb); err != nil {
				return err
			}
			if err := d.plane.lp.decodeMB(mb); err != nil {
				return err
			}
			if err := d.plane.hp.decodeCBPHP(mb); err != nil {
				return err
			}
			if err := d.plane.hp.decodeHPFlex(mb, true, true, 0); err != nil {
				return err
			}
		}
	}
	d.br.discardRemainderBits()
	return nil
}

func (d *grayDecoder) firstLevelInverseTransform() {
	for mby := 0; mby < d.header.MBHeight; mby++ {
		for mbx := 0; mbx < d.header.MBWidth; mbx++ {
			mb := d.mbs[mbx][mby]
			var dclp [16]int
			for j := 0; j < 16; j++ {
				dclp[j] = mb.MBBuffer[j*16]
			}
			dclp = strIDCT4x4Stage2(dclp)
			for j := 0; j < 16; j++ {
				mb.MBBuffer[j*16] = dclp[j]
			}
		}
	}
}

func (d *grayDecoder) secondLevelInverseTransform() {
	for mby := 0; mby < d.header.MBHeight; mby++ {
		for mbx := 0; mbx < d.header.MBWidth; mbx++ {
			mb := d.mbs[mbx][mby]
			for j := 0; j < 16; j++ {
				var coeff [16]int
				base := j * 16
				copy(coeff[:], mb.MBBuffer[base:base+16])
				coeff = strIDCT4x4Stage1(coeff)
				copy(mb.MBBuffer[base:base+16], coeff[:])
			}
		}
	}
}

func (d *grayDecoder) secondLevelCoefficientCombination() {
	d.imagePlane = make([]int, d.paddedWidth*d.paddedHeight)
	for mby := 0; mby < d.header.MBHeight; mby++ {
		mbyy := mby << 4
		for mbx := 0; mbx < d.header.MBWidth; mbx++ {
			mbxx := mbx << 4
			mb := d.mbs[mbx][mby]
			for by := 0; by < 4; by++ {
				byX4 := mbyy + (by << 2)
				byX16 := by << 4
				for bx := 0; bx < 4; bx++ {
					bxX4 := mbxx + (bx << 2)
					bxX64 := bx << 6
					for py := 0; py < 4; py++ {
						pyX4 := py << 2
						for px := 0; px < 4; px++ {
							x := bxX4 + px
							y := byX4 + py
							d.imagePlane[y*d.paddedWidth+x] = mb.MBBuffer[byX16+bxX64+mbPixelMap[px+pyX4]]
						}
					}
				}
			}
		}
	}
}

func (d *grayDecoder) outputFormatting() {
	d.addBias()
	d.computeScaling()
	d.clip()
}

func (d *grayDecoder) addBias() {
	bias := 1 << 7
	bias <<= 3
	for i := range d.imagePlane {
		d.imagePlane[i] += bias
	}
}

func (d *grayDecoder) computeScaling() {
	for i := range d.imagePlane {
		d.imagePlane[i] = (d.imagePlane[i] + 3) >> 3
	}
}

func (d *grayDecoder) clip() {
	for i := range d.imagePlane {
		d.imagePlane[i] = clip(d.imagePlane[i], 0, 255)
	}
}

func (d *grayDecoder) toImage() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, d.header.ImageWidth, d.header.ImageHeight))
	for y := 0; y < d.header.ImageHeight; y++ {
		for x := 0; x < d.header.ImageWidth; x++ {
			srcX := x + d.header.ExtraLeft
			srcY := y + d.header.ExtraTop
			img.Pix[y*img.Stride+x] = uint8(d.imagePlane[srcY*d.paddedWidth+srcX])
		}
	}
	return img
}

func (b *dcBand) decodeMB(mb *grayMB) error {
	if mb.InitializeContext {
		b.absLevelLum.initializeTable1()
		b.model.initialize(bandDC)
	}
	bAbsLevel, err := b.plane.decoder.br.readBits(1)
	if err != nil {
		return err
	}
	lapMean := 0
	if bAbsLevel != 0 {
		lapMean = 1
	}
	iDC, err := b.decodeDC(bAbsLevel != 0)
	if err != nil {
		return err
	}
	b.model.update(lapMean, bandDC)
	if mb.ResetContext {
		b.absLevelLum.adaptTable1()
	}
	mb.MbDCLP[0] = iDC

	switch {
	case mb.IsMBLeftEdgeofTileFlag && mb.IsMBTopEdgeofTileFlag:
		mb.MBDCMode = noPrediction
	case mb.IsMBLeftEdgeofTileFlag:
		mb.MBDCMode = predictFromTop
	case mb.IsMBTopEdgeofTileFlag:
		mb.MBDCMode = predictFromLeft
	default:
		iLeft := mb.left.MbDCLP[0]
		iTop := mb.top.MbDCLP[0]
		iTopLeft := mb.top.left.MbDCLP[0]
		strHor := abs(iTopLeft - iLeft)
		strVer := abs(iTopLeft - iTop)
		if strHor*4 < strVer {
			mb.MBDCMode = predictFromTop
		} else if strVer*4 < strHor {
			mb.MBDCMode = predictFromLeft
		} else {
			mb.MBDCMode = predictFromTopLeft
		}
	}
	switch mb.MBDCMode {
	case predictFromLeft:
		mb.MbDCLP[0] += mb.left.MbDCLP[0]
	case predictFromTop:
		mb.MbDCLP[0] += mb.top.MbDCLP[0]
	case predictFromTopLeft:
		mb.MbDCLP[0] += (mb.top.MbDCLP[0] + mb.left.MbDCLP[0]) >> 1
	}
	mb.MBBuffer[16*ict4x4InvPermArr[0]] = mb.MbDCLP[0] * b.plane.dcScale
	return nil
}

func (b *dcBand) decodeDC(absLevel bool) (int, error) {
	iDC := 0
	if absLevel {
		level, err := decodeAbsLevel(b.plane.decoder.br, &b.absLevelLum, false)
		if err != nil {
			return 0, err
		}
		iDC = level - 1
	}
	if b.model.MBits > 0 {
		ref, err := b.plane.decoder.br.readBits(b.model.MBits)
		if err != nil {
			return 0, err
		}
		iDC = (iDC << b.model.MBits) | ref
	}
	return signOptional(b.plane.decoder.br, iDC)
}

func (b *lpBand) decodeMB(mb *grayMB) error {
	if mb.InitializeContext {
		b.firstLum.initializeTable2()
		b.indLum0.initializeTable2()
		b.indLum1.initializeTable2()
		b.absLevel0.initializeTable1()
		b.absLevel1.initializeTable1()
		b.lowpassScan = newAdaptiveScan(lowpassScanOrder)
		b.model.initialize(bandLP)
	}
	if mb.ResetTotals {
		b.lowpassScan.reset()
	}

	cbplpBit, err := b.plane.decoder.br.readBits(1)
	if err != nil {
		return err
	}
	var lpInput [16]int
	lapMean := 0
	if cbplpBit != 0 {
		block, err := decodeBlock(b.plane.decoder.br, bandLP, 1, false, &b.firstLum, &b.indLum0, &b.indLum1, &b.absLevel0, &b.absLevel1)
		if err != nil {
			return err
		}
		lapMean += len(block)
		i := 1
		for _, pair := range block {
			i += pair.run
			lpInput[b.lowpassScan.translate(i)] = pair.level
			b.lowpassScan.adapt(i)
			i++
		}
	}
	if b.model.MBits > 0 {
		for k := 1; k < 16; k++ {
			ref, err := b.plane.decoder.br.readBits(b.model.MBits)
			if err != nil {
				return err
			}
			switch {
			case lpInput[k] > 0:
				lpInput[k] = (lpInput[k] << b.model.MBits) + ref
			case lpInput[k] < 0:
				lpInput[k] = (lpInput[k] << b.model.MBits) - ref
			default:
				lpInput[k], err = signOptional(b.plane.decoder.br, ref)
				if err != nil {
					return err
				}
			}
		}
	}

	b.model.update(lapMean, bandLP)
	if mb.ResetContext {
		b.firstLum.adaptTable2(4)
		b.indLum0.adaptTable2(3)
		b.indLum1.adaptTable2(3)
		b.absLevel0.adaptTable1()
		b.absLevel1.adaptTable1()
	}
	for j := 1; j < 16; j++ {
		mb.MbDCLP[j] = lpInput[j]
	}

	switch {
	case mb.MBDCMode == predictFromLeft && mb.left != nil && mb.MBQPIndexLP == mb.left.MBQPIndexLP:
		mb.MBLPMode = predictFromLeft
	case mb.MBDCMode == predictFromTop && mb.top != nil && mb.MBQPIndexLP == mb.top.MBQPIndexLP:
		mb.MBLPMode = predictFromTop
	default:
		mb.MBLPMode = noPrediction
	}

	if mb.MBLPMode == predictFromLeft {
		for _, j := range []int{1, 2, 3} {
			mb.MbDCLP[j] += mb.left.MbDCLP[j]
		}
	} else if mb.MBLPMode == predictFromTop {
		for _, j := range []int{4, 8, 12} {
			mb.MbDCLP[j] += mb.top.MbDCLP[j]
		}
	}

	for j := 1; j < 16; j++ {
		mb.MBBuffer[16*ict4x4InvPermArr[j]] = mb.MbDCLP[j] * b.plane.lpScale
	}
	return nil
}

func (b *hpBand) decodeCBPHP(mb *grayMB) error {
	if mb.InitializeContext {
		b.numCBPHP.initializeTable1()
		b.numBlkCBPHP.initializeTable1()
	}
	num, err := b.plane.decoder.br.huff(numCBPHP[b.numCBPHP.TableIndex])
	if err != nil {
		return err
	}
	b.numCBPHP.DiscrimVal1 += [5]int{0, -1, 0, 1, 1}[num]
	iCBPHP, err := b.refineCBPHP(num)
	if err != nil {
		return err
	}

	iDiffCBPHP := 0
	for iBlock := 0; iBlock < 4; iBlock++ {
		if (iCBPHP & (1 << iBlock)) == 0 {
			continue
		}
		numBlk, err := b.plane.decoder.br.huff(numBlkCBPHP1[b.numBlkCBPHP.TableIndex])
		if err != nil {
			return err
		}
		b.numBlkCBPHP.DiscrimVal1 += [5]int{0, -1, 0, 1, 1}[numBlk]
		iVal := numBlk + 1
		iBlkCBPHP := 0
		if iVal >= 6 {
			ref, err := b.plane.decoder.br.huff(hbin(map[string]int{"1": 0, "01": 1, "00": 2}))
			if err != nil {
				return err
			}
			iBlkCBPHP = 0x10 * (ref + 1)
			if iVal >= 9 {
				inc, err := b.plane.decoder.br.huff(hbin(map[string]int{"1": 0, "01": 1, "00": 2}))
				if err != nil {
					return err
				}
				iVal += inc
			}
			iVal -= 6
		}
		iOff := [6]int{0, 4, 2, 8, 12, 1}
		iFLC := [6]int{0, 2, 1, 2, 2, 0}
		iOut := [16]int{0, 15, 3, 12, 1, 2, 4, 8, 5, 6, 9, 10, 7, 11, 13, 14}
		iCode := iOff[iVal]
		if iFLC[iVal] != 0 {
			extra, err := b.plane.decoder.br.readBits(iFLC[iVal])
			if err != nil {
				return err
			}
			iCode += extra
		}
		iBlkCBPHP += iOut[iCode]
		iDiffCBPHP |= iBlkCBPHP << (iBlock * 4)
	}

	if mb.InitializeContext {
		b.cbphpState = 0
		b.countOnes = -4
		b.countZeroes = 4
	}
	mb.MBCBPHP = b.predCBPHP(iDiffCBPHP, mb)
	return nil
}

func (b *hpBand) refineCBPHP(iNum int) (int, error) {
	switch iNum {
	case 2:
		return b.plane.decoder.br.huff(refCBPHP1)
	case 1:
		scale, err := b.plane.decoder.br.readBits(2)
		if err != nil {
			return 0, err
		}
		return 1 << scale, nil
	case 3:
		scale, err := b.plane.decoder.br.readBits(2)
		if err != nil {
			return 0, err
		}
		return 0x0F ^ (1 << scale), nil
	case 4:
		return 0x0F, nil
	default:
		return 0, nil
	}
}

func (b *hpBand) predCBPHP(iDiffCBPHP int, mb *grayMB) int {
	iCBPHP := iDiffCBPHP
	switch b.cbphpState {
	case 0:
		if mb.IsMBLeftEdgeofTileFlag {
			if mb.IsMBTopEdgeofTileFlag {
				iCBPHP ^= 1
			} else {
				iCBPHP ^= (mb.top.MBCBPHP >> 10) & 1
			}
		} else {
			iCBPHP ^= (mb.left.MBCBPHP >> 5) & 1
		}
		iCBPHP ^= 0x02 & (iCBPHP << 1)
		iCBPHP ^= 0x10 & (iCBPHP << 3)
		iCBPHP ^= 0x20 & (iCBPHP << 1)
		iCBPHP ^= (iCBPHP & 0x33) << 2
		iCBPHP ^= (iCBPHP & 0x00CC) << 6
		iCBPHP ^= (iCBPHP & 0x3300) << 2
	case 2:
		iCBPHP ^= 0x0000FFFF
	}

	nOrig := numOnes(iCBPHP)
	b.countOnes += nOrig - 3
	b.countOnes = clip(b.countOnes, -16, 15)
	b.countZeroes += (16 - nOrig) - 3
	b.countZeroes = clip(b.countZeroes, -16, 15)
	if b.countOnes < 0 {
		if b.countOnes < b.countZeroes {
			b.cbphpState = 1
		} else {
			b.cbphpState = 2
		}
	} else if b.countZeroes < 0 {
		b.cbphpState = 2
	} else {
		b.cbphpState = 0
	}
	return iCBPHP
}

func (b *hpBand) decodeHPFlex(mb *grayMB, doHP, doFlex bool, trimFlexBits int) error {
	if doHP {
		if mb.InitializeContext {
			b.firstLum.initializeTable2()
			b.indLum0.initializeTable2()
			b.indLum1.initializeTable2()
			b.absLevel0.initializeTable1()
			b.absLevel1.initializeTable1()
			b.highpassHor = newAdaptiveScan(highpassHorOrder)
			b.highpassVer = newAdaptiveScan(highpassVerOrder)
			b.model.initialize(bandHP)
		}
		if mb.ResetTotals {
			b.highpassHor.reset()
			b.highpassVer.reset()
		}
		mb.MBHPMode = calcHPPredMode(mb)
	}
	iModelBits := 0
	if doFlex {
		if doHP {
			iModelBits = b.model.MBits
		} else {
			iModelBits = mb.ModelBitsMBHP
		}
	}
	iCBPHP := mb.MBCBPHP
	lapMean := 0
	for _, iBlock := range iHierScanOrder {
		if doHP {
			adScan := &b.highpassHor
			if mb.MBHPMode == predictFromTop {
				adScan = &b.highpassVer
			}
			nnz, err := b.decodeBlockAdaptive((iCBPHP&1) != 0, iBlock, adScan, mb)
			if err != nil {
				return err
			}
			lapMean += nnz
			iCBPHP >>= 1
		}
		if doFlex && b.plane.flexbitsPresent {
			if err := b.blockFlexbits(iBlock, iModelBits, trimFlexBits, mb); err != nil {
				return err
			}
		}
	}
	if doHP {
		mb.ModelBitsMBHP = b.model.MBits
		b.model.update(lapMean, bandHP)
		if mb.ResetContext {
			b.firstLum.adaptTable2(4)
			b.indLum0.adaptTable2(3)
			b.indLum1.adaptTable2(3)
			b.absLevel0.adaptTable1()
			b.absLevel1.adaptTable1()
			b.numCBPHP.adaptTable1()
			b.numBlkCBPHP.adaptTable1()
		}
	}
	if (doHP && b.plane.flexbitsPresent) || doFlex {
		b.hpTransformCoefficientDecoding(mb)
	}
	return nil
}

func (b *hpBand) decodeBlockAdaptive(noSkip bool, iBlock int, scan *adaptiveScan, mb *grayMB) (int, error) {
	if !noSkip {
		return 0, nil
	}
	block, err := decodeBlock(b.plane.decoder.br, bandHP, 1, false, &b.firstLum, &b.indLum0, &b.indLum1, &b.absLevel0, &b.absLevel1)
	if err != nil {
		return 0, err
	}
	iLocation := 1
	iNumNonZero := 0
	for _, pair := range block {
		iLocation += pair.run
		if iLocation < 1 || iLocation > 15 {
			return 0, fmt.Errorf("HP block location out of range")
		}
		mb.HPInputVLC[iBlock][scan.translate(iLocation)] = pair.level
		scan.adapt(iLocation)
		iLocation++
		iNumNonZero++
	}
	return iNumNonZero, nil
}

func (b *hpBand) blockFlexbits(iBlock, modelBits, trimFlexBits int, mb *grayMB) error {
	bitsLeft := modelBits - trimFlexBits
	if bitsLeft <= 0 {
		return nil
	}
	for _, n := range transposeFlex[1:] {
		ref, err := b.plane.decoder.br.readBits(bitsLeft)
		if err != nil {
			return err
		}
		vlc := mb.HPInputVLC[iBlock][n]
		var coeff int
		switch {
		case vlc > 0:
			coeff = ref
		case vlc < 0:
			coeff = -ref
		default:
			coeff, err = signOptional(b.plane.decoder.br, ref)
			if err != nil {
				return err
			}
		}
		mb.HPInputFlex[iBlock][n] = coeff << trimFlexBits
	}
	return nil
}

func (b *hpBand) hpTransformCoefficientDecoding(mb *grayMB) {
	for blk := 0; blk < 16; blk++ {
		for j := 1; j < 16; j++ {
			mb.MBBuffer[16*blk+j] = ((mb.HPInputVLC[blk][j] << mb.ModelBitsMBHP) + mb.HPInputFlex[blk][j]) * b.plane.hpScale
		}
	}
	switch mb.MBHPMode {
	case predictFromTop:
		for _, blk := range []int{1, 2, 3, 5, 6, 7, 9, 10, 11, 13, 14, 15} {
			for _, k := range []int{2, 10, 9} {
				mb.MBBuffer[16*blk+k] += mb.MBBuffer[16*(blk-1)+k]
			}
		}
	case predictFromLeft:
		for blk := 4; blk < 16; blk++ {
			for _, k := range []int{1, 5, 6} {
				mb.MBBuffer[16*blk+k] += mb.MBBuffer[16*(blk-4)+k]
			}
		}
	}
}

func calcHPPredMode(mb *grayMB) int {
	strHor := abs(mb.MbDCLP[1]) + abs(mb.MbDCLP[2]) + abs(mb.MbDCLP[3])
	strVer := abs(mb.MbDCLP[4]) + abs(mb.MbDCLP[8]) + abs(mb.MbDCLP[12])
	if strHor*4 < strVer {
		return predictFromTop
	}
	if strVer*4 < strHor {
		return predictFromLeft
	}
	return noPrediction
}

type runLevel struct {
	run   int
	level int
}

func decodeBlock(br *bitReader, band, location int, chroma bool, firstVLC, ind0, ind1, abs0, abs1 *adaptiveVLC) ([]runLevel, error) {
	if location < 0 || location > 15 {
		return nil, fmt.Errorf("decode block start %d out of range", location)
	}
	runIsZero, levelNot1, nextImmediate, nextAfterRun, err := decodeFirstIndex(br, firstVLC)
	if err != nil {
		return nil, err
	}
	context := boolToInt(runIsZero && nextImmediate)

	sign, err := br.readBits(1)
	if err != nil {
		return nil, err
	}
	level := 1
	if levelNot1 {
		level, err = decodeAbsLevel(br, chooseAbsVLC(abs0, abs1, context), chroma)
		if err != nil {
			return nil, err
		}
	}
	level = signedValue(level, sign != 0)

	run := 0
	if !runIsZero {
		run, err = decodeRun(br, 15-location)
		if err != nil {
			return nil, err
		}
	}
	block := []runLevel{{run: run, level: level}}
	loc := location + run + 1

	for nextImmediate || nextAfterRun {
		run = 0
		if !nextImmediate {
			run, err = decodeRun(br, 15-loc)
			if err != nil {
				return nil, err
			}
		}
		loc += run + 1
		if loc > 16 {
			return nil, fmt.Errorf("decoded block location out of range")
		}
		levelNot1, nextImmediate, nextAfterRun, err = decodeIndex(br, band, loc, context, ind0, ind1)
		if err != nil {
			return nil, err
		}
		if !nextImmediate {
			context = 0
		}
		sign, err = br.readBits(1)
		if err != nil {
			return nil, err
		}
		level = 1
		if levelNot1 {
			level, err = decodeAbsLevel(br, chooseAbsVLC(abs0, abs1, context), chroma)
			if err != nil {
				return nil, err
			}
		}
		block = append(block, runLevel{run: run, level: signedValue(level, sign != 0)})
	}
	return block, nil
}

func chooseAbsVLC(abs0, abs1 *adaptiveVLC, context int) *adaptiveVLC {
	if context != 0 {
		return abs1
	}
	return abs0
}

func decodeFirstIndex(br *bitReader, vlc *adaptiveVLC) (bool, bool, bool, bool, error) {
	value, err := br.huff(firstIndex[vlc.TableIndex])
	if err != nil {
		return false, false, false, false, err
	}
	vlc.DiscrimVal1 += firstIndexDelta[vlc.DeltaTableIndex][value]
	vlc.DiscrimVal2 += firstIndexDelta[vlc.Delta2TableIndex][value]
	return (value & 0x01) != 0, (value&0x02) != 0, (value&0x04) != 0, (value >> 3) != 0, nil
}

func decodeIndex(br *bitReader, band, location, context int, ind0, ind1 *adaptiveVLC) (bool, bool, bool, error) {
	vlc := ind0
	if context != 0 {
		vlc = ind1
	}
	var value int
	var err error
	switch {
	case location < 15:
		value, err = br.huff(indexA[vlc.TableIndex])
		if err != nil {
			return false, false, false, err
		}
		vlc.DiscrimVal1 += [6]int(index1Delta[vlc.DeltaTableIndex])[value]
		vlc.DiscrimVal2 += [6]int(index1Delta[vlc.Delta2TableIndex])[value]
	case location == 15:
		value, err = br.huff(indexB)
		if err != nil {
			return false, false, false, err
		}
	default:
		value, err = br.readBits(1)
		if err != nil {
			return false, false, false, err
		}
	}
	return (value & 0x01) != 0, (value&0x02) != 0, (value >> 2) != 0, nil
}

func decodeRun(br *bitReader, maxRun int) (int, error) {
	if maxRun < 1 || maxRun > 14 {
		return 0, fmt.Errorf("invalid max run %d", maxRun)
	}
	if maxRun < 5 {
		if maxRun == 1 {
			return 1, nil
		}
		return br.huff(runValue[maxRun])
	}
	iRunBinx := [10]int{10, 10, 5, 5, 5, 5, 0, 0, 0, 0}
	iRunFixed := [15]int{0, 0, 1, 1, 3, 0, 0, 1, 1, 2, 0, 0, 0, 0, 1}
	iRemap := [15]int{1, 2, 3, 5, 7, 1, 2, 3, 5, 7, 1, 2, 3, 4, 5}
	idx, err := br.huff(runIndex)
	if err != nil {
		return 0, err
	}
	idx += iRunBinx[maxRun-5]
	run := iRemap[idx]
	if fixed := iRunFixed[idx]; fixed != 0 {
		ref, err := br.readBits(fixed)
		if err != nil {
			return 0, err
		}
		run += ref
	}
	if run < 1 || run > maxRun {
		return 0, fmt.Errorf("run %d out of range 1-%d", run, maxRun)
	}
	return run, nil
}

func decodeAbsLevel(br *bitReader, vlc *adaptiveVLC, chroma bool) (int, error) {
	value, err := br.huff(absLevelIndex[vlc.TableIndex])
	if err != nil {
		return 0, err
	}
	vlc.DiscrimVal1 += absLevelIndexDelta[value]
	remap := [6]int{2, 3, 4, 6, 10, 14}
	fixedLen := [6]int{0, 0, 1, 2, 2, 2}
	if value < 6 {
		level := remap[value]
		if fixed := fixedLen[value]; fixed > 0 {
			ref, err := br.readBits(fixed)
			if err != nil {
				return 0, err
			}
			level += ref
		}
		return level, nil
	}
	fixed, err := br.readBits(4)
	if err != nil {
		return 0, err
	}
	fixed += 4
	if fixed == 19 {
		extra, err := br.readBits(2)
		if err != nil {
			return 0, err
		}
		fixed += extra
		if fixed == 22 {
			extra2, err := br.readBits(3)
			if err != nil {
				return 0, err
			}
			fixed += extra2
		}
	}
	ref, err := br.readBits(fixed)
	if err != nil {
		return 0, err
	}
	return 2 + (1 << fixed) + ref, nil
}

func (m *grayModel) initialize(band int) {
	m.MState = 0
	m.MBits = (2 - band) * 4
}

func (m *grayModel) update(lapMean int, band int) {
	lapMean *= firstModelWeight[band]
	delta := (lapMean - 70) >> 2
	if delta <= -8 {
		delta += 4
		if delta < -16 {
			delta = -16
		}
		m.MState += delta
		if m.MState < -8 {
			if m.MBits == 0 {
				m.MState = -8
			} else {
				m.MState = 0
				m.MBits--
			}
		}
	} else if delta >= 8 {
		delta -= 4
		if delta > 15 {
			delta = 15
		}
		m.MState += delta
		if m.MState > 8 {
			if m.MBits >= 15 {
				m.MBits = 15
				m.MState = 8
			} else {
				m.MState = 0
				m.MBits++
			}
		}
	}
}

func (v *adaptiveVLC) initializeTable1() {
	v.TableIndex, v.DeltaTableIndex, v.DiscrimVal1 = 0, 0, 0
}

func (v *adaptiveVLC) adaptTable1() {
	if v.DiscrimVal1 < -8 && v.TableIndex != 0 {
		v.TableIndex--
		v.DiscrimVal1 = 0
	} else if v.DiscrimVal1 > 8 && v.TableIndex != 1 {
		v.TableIndex++
		v.DiscrimVal1 = 0
	} else {
		v.DiscrimVal1 = clip(v.DiscrimVal1, -64, 64)
	}
}

func (v *adaptiveVLC) initializeTable2() {
	v.DeltaTableIndex, v.DiscrimVal1, v.DiscrimVal2 = 0, 0, 0
	v.TableIndex, v.Delta2TableIndex = 1, 1
}

func (v *adaptiveVLC) adaptTable2(maxTable int) {
	changed := false
	if v.DiscrimVal1 < -8 && v.TableIndex != 0 {
		v.TableIndex--
		changed = true
	} else if v.DiscrimVal2 > 8 && v.TableIndex != maxTable {
		v.TableIndex++
		changed = true
	}
	if changed {
		v.DiscrimVal1, v.DiscrimVal2 = 0, 0
		if v.TableIndex == maxTable {
			v.DeltaTableIndex, v.Delta2TableIndex = v.TableIndex-1, v.TableIndex-1
		} else if v.TableIndex == 0 {
			v.DeltaTableIndex, v.Delta2TableIndex = 0, 0
		} else {
			v.DeltaTableIndex, v.Delta2TableIndex = v.TableIndex-1, v.TableIndex
		}
	} else {
		v.DiscrimVal1 = clip(v.DiscrimVal1, -64, 64)
		v.DiscrimVal2 = clip(v.DiscrimVal2, -64, 64)
	}
}

func newAdaptiveScan(order [16]int) adaptiveScan {
	s := adaptiveScan{order: order}
	s.reset()
	return s
}

func (s *adaptiveScan) reset() {
	s.totals = [16]int{0, 32, 30, 28, 26, 24, 22, 20, 18, 16, 14, 12, 10, 8, 6, 4}
}

func (s *adaptiveScan) translate(i int) int {
	return s.order[i]
}

func (s *adaptiveScan) adapt(i int) {
	s.totals[i]++
	if i > 1 && s.totals[i] > s.totals[i-1] {
		s.order[i], s.order[i-1] = s.order[i-1], s.order[i]
		s.totals[i], s.totals[i-1] = s.totals[i-1], s.totals[i]
	}
}

func (r *bitReader) huff(table map[int]int) (int, error) {
	k := 1
	for k <= 0xff {
		bit, err := r.readBits(1)
		if err != nil {
			return 0, err
		}
		k = (k << 1) + bit
		if value, ok := table[k]; ok {
			return value, nil
		}
	}
	return 0, fmt.Errorf("huffman decode failed")
}

func (r *bitReader) discardRemainderBits() {
	r.bitsRemaining = 0
	r.remainder = 0
}

func signOptional(br *bitReader, value int) (int, error) {
	if value == 0 {
		return 0, nil
	}
	sign, err := br.readBits(1)
	if err != nil {
		return 0, err
	}
	return signedValue(value, sign != 0), nil
}

func signedValue(value int, sign bool) int {
	if sign {
		return -value
	}
	return value
}

func hbin(stbl map[string]int) map[int]int {
	out := make(map[int]int, len(stbl))
	for k, v := range stbl {
		acc := 1
		for _, ch := range k {
			acc <<= 1
			if ch == '1' {
				acc++
			}
		}
		out[acc] = v
	}
	return out
}

func numOnes(x int) int {
	value := 0
	for x != 0 {
		value += x & 1
		x >>= 1
	}
	return value
}

func clip(x, low, high int) int {
	if x < low {
		return low
	}
	if x > high {
		return high
	}
	return x
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func strIDCT4x4Stage1(coeff [16]int) [16]int {
	a, b, c, d := strDCT2x2up([4]int{coeff[0], coeff[1], coeff[2], coeff[3]})
	coeff[0], coeff[1], coeff[2], coeff[3] = a, b, c, d
	a, b, c, d = invOdd([4]int{coeff[5], coeff[4], coeff[7], coeff[6]})
	coeff[5], coeff[4], coeff[7], coeff[6] = a, b, c, d
	a, b, c, d = invOdd([4]int{coeff[10], coeff[8], coeff[11], coeff[9]})
	coeff[10], coeff[8], coeff[11], coeff[9] = a, b, c, d
	a, b, c, d = invOddOdd([4]int{coeff[15], coeff[14], coeff[13], coeff[12]})
	coeff[15], coeff[14], coeff[13], coeff[12] = a, b, c, d
	return fourButterfly(coeff, [][4]int{{0, 4, 8, 12}, {1, 5, 9, 13}, {2, 6, 10, 14}, {3, 7, 11, 15}})
}

func strIDCT4x4Stage2(coeff [16]int) [16]int {
	a, b, c, d := invOdd([4]int{coeff[2], coeff[3], coeff[6], coeff[7]})
	coeff[2], coeff[3], coeff[6], coeff[7] = a, b, c, d
	a, b, c, d = invOdd([4]int{coeff[8], coeff[12], coeff[9], coeff[13]})
	coeff[8], coeff[12], coeff[9], coeff[13] = a, b, c, d
	a, b, c, d = invOddOdd([4]int{coeff[10], coeff[14], coeff[11], coeff[15]})
	coeff[10], coeff[14], coeff[11], coeff[15] = a, b, c, d
	a, b, c, d = strDCT2x2up([4]int{coeff[0], coeff[4], coeff[1], coeff[5]})
	coeff[0], coeff[4], coeff[1], coeff[5] = a, b, c, d
	return fourButterfly(coeff, [][4]int{{0, 12, 3, 15}, {4, 8, 7, 11}, {1, 13, 2, 14}, {5, 9, 6, 10}})
}

func strDCT2x2up(coeff [4]int) (int, int, int, int) {
	a, b, c, d := coeff[0], coeff[1], coeff[2], coeff[3]
	a += d
	b -= c
	t := (a - b + 1) >> 1
	c = t - d
	d = t - coeff[2]
	a -= d
	b += c
	return a, b, c, d
}

func invOdd(coeff [4]int) (int, int, int, int) {
	a, b, c, d := coeff[0], coeff[1], coeff[2], coeff[3]
	b += d
	a -= c
	d -= b >> 1
	c += (a + 1) >> 1
	a, b = irotate2(a, b)
	c, d = irotate2(c, d)
	c -= (b + 1) >> 1
	d = ((a + 1) >> 1) - d
	b += c
	a -= d
	return a, b, c, d
}

func invOddOdd(coeff [4]int) (int, int, int, int) {
	a, b, c, d := coeff[0], coeff[1], coeff[2], coeff[3]
	d += a
	c -= b
	t1 := d >> 1
	a -= t1
	t2 := c >> 1
	b += t2
	a -= (b*3 + 3) >> 3
	b += (a*3 + 3) >> 2
	a -= (b*3 + 4) >> 3
	b -= t2
	a += t1
	c += b
	d -= a
	return a, -b, -c, d
}

func irotate2(a, b int) (int, int) {
	a -= (b*3 + 4) >> 3
	b += (a*3 + 4) >> 3
	return a, b
}

func fourButterfly(coeff [16]int, order [][4]int) [16]int {
	for _, o := range order {
		a, b, c, d := strDCT2x2dn([4]int{coeff[o[0]], coeff[o[1]], coeff[o[2]], coeff[o[3]]})
		coeff[o[0]], coeff[o[1]], coeff[o[2]], coeff[o[3]] = a, b, c, d
	}
	return coeff
}

func strDCT2x2dn(coeff [4]int) (int, int, int, int) {
	a, b, c, d := coeff[0], coeff[1], coeff[2], coeff[3]
	a += d
	b -= c
	t := (a - b) >> 1
	c = t - d
	d = t - coeff[2]
	a -= d
	b += c
	return a, b, c, d
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = math.MaxInt // keep math import if further decode phases extend this file
