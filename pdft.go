package pdft

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/signintech/gopdf"
)

//ErrAddSameFontName add same font name
var ErrAddSameFontName = errors.New("add same font name")

//ErrFontNameNotFound font name not found
var ErrFontNameNotFound = errors.New("font name not found")

//Left left
const Left = gopdf.Left //001000
//Top top
const Top = gopdf.Top //000100
//Right right
const Right = gopdf.Right //000010
//Bottom bottom
const Bottom = gopdf.Bottom //000001
//Center center
const Center = gopdf.Center //010000
//Middle middle
const Middle = gopdf.Middle //100000

//PDFt inject text to pdf
type PDFt struct {
	pdf           PDFData
	fontDatas     map[string]*PDFFontData
	pdfImgs       []*PDFImageData
	curr          current
	contenters    []Contenter
	pdfProtection *gopdf.PDFProtection
}

type current struct {
	fontName  string
	fontStyle string
	fontSize  int
	lineWidth float64
}

func pageHeight() float64 {
	return 841.89
}

func (i *PDFt) protection() *gopdf.PDFProtection {
	return i.pdfProtection
}

//ShowCellBorder  show cell of border
func (i *PDFt) ShowCellBorder(isShow bool) {
	var clw ContentLineStyle
	if isShow {
		clw.width = 0.1
		clw.lineType = "dotted"
		i.curr.lineWidth = 0.1
	} else {
		clw.width = 0.0
		clw.lineType = ""
		i.curr.lineWidth = 0.0
	}
	i.contenters = append(i.contenters, &clw)
}

//Open open pdf file
func (i *PDFt) Open(filepath string) error {
	//init
	i.fontDatas = make(map[string]*PDFFontData)
	i.curr.lineWidth = 1.0
	//open
	f, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	err = PDFParse(f, &i.pdf)
	if err != nil {
		return err
	}
	//fmt.Printf("%s\n", i.pdf.hash())
	i.ShowCellBorder(false)

	return nil
}

//Insert insert text in to pdf
func (i *PDFt) Insert(text string, pageNum int, x float64, y float64, w float64, h float64, align int) error {

	var ct ContentText
	ct.text = text
	ct.fontName = i.curr.fontName
	ct.fontStyle = i.curr.fontStyle
	ct.fontSize = i.curr.fontSize
	ct.pageNum = pageNum
	ct.x = x
	ct.y = y
	ct.w = w
	ct.h = h
	ct.align = align
	ct.lineWidth = i.curr.lineWidth
	ct.setProtection(i.protection())
	if _, have := i.fontDatas[ct.fontName]; !have {
		return ErrFontNameNotFound
	}
	ct.pdfFontData = i.fontDatas[ct.fontName]
	i.contenters = append(i.contenters, &ct)
	return i.fontDatas[ct.fontName].addChars(text)
}

//InsertImgBase64 insert img base 64
func (i *PDFt) InsertImgBase64(base64str string, pageNum int, x float64, y float64, w float64, h float64) error {

	var pdfimg PDFImageData
	err := pdfimg.setImgBase64(base64str)
	if err != nil {
		return err
	}
	i.pdfImgs = append(i.pdfImgs, &pdfimg)
	//fmt.Printf("-->%d\n", len(i.pdfImgs))

	var ct contentImgBase64
	ct.pageNum = pageNum
	ct.x = x
	ct.y = y
	ct.h = h
	ct.w = w
	ct.refPdfimg = &pdfimg //i.pdfImgs[len(i.pdfImgs)-1]
	i.contenters = append(i.contenters, &ct)
	return nil
}

//AddFont add ttf font
func (i *PDFt) AddFont(name string, ttfpath string) error {

	if _, have := i.fontDatas[name]; have {
		return ErrAddSameFontName
	}

	fontData, err := PDFParseFont(ttfpath, name)
	if err != nil {
		return err
	}

	i.fontDatas[name] = fontData
	return nil
}

//SetFont set font
func (i *PDFt) SetFont(name string, style string, size int) error {

	if _, have := i.fontDatas[name]; !have {
		return ErrFontNameNotFound
	}
	i.curr.fontName = name
	i.curr.fontStyle = style
	i.curr.fontSize = size
	return nil
}

//Save save output pdf
func (i *PDFt) Save(filepath string) error {
	var buff bytes.Buffer
	err := i.SaveTo(&buff)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath, buff.Bytes(), 0644)
	if err != nil {
		return err
	}
	return nil
}

//SaveTo save pdf to io.Writer
func (i *PDFt) SaveTo(w io.Writer) error {

	newpdf, lastID, err := i.build()
	if err != nil {
		return err
	}

	buff, err := i.toStream(newpdf, lastID)
	if err != nil {
		return err
	}
	_, err = buff.WriteTo(w)
	if err != nil {
		return err
	}
	return nil
}

func (i *PDFt) build() (*PDFData, int, error) {

	var err error
	nextID := i.pdf.maxID()
	for _, fontData := range i.fontDatas {
		nextID++
		fontData.setStartID(nextID)
		nextID, err = fontData.build()
		if err != nil {
			return nil, 0, err
		}
	}

	newpdf := i.pdf //copy

	err = newpdf.injectFontsToPDF(i.fontDatas)
	if err != nil {
		return nil, 0, err
	}

	//ยัด subsetfont obj กลับไป
	for _, fontData := range i.fontDatas {

		var fontobj, cidObj, unicodeMapObj, fontDescObj, dictionaryObj PDFObjData

		fontobj.objID = fontData.fontID
		fontobj.data = fontData.fontStream.Bytes()

		cidObj.objID = fontData.cidID
		cidObj.data = fontData.cidStream.Bytes()

		unicodeMapObj.objID = fontData.unicodeMapID
		unicodeMapObj.data = fontData.unicodeMapStream.Bytes()

		fontDescObj.objID = fontData.fontDescID
		fontDescObj.data = fontData.fontDescStream.Bytes()

		dictionaryObj.objID = fontData.dictionaryID
		dictionaryObj.data = fontData.dictionaryStream.Bytes()

		newpdf.put(fontobj)
		newpdf.put(cidObj)
		newpdf.put(unicodeMapObj)
		newpdf.put(fontDescObj)
		newpdf.put(dictionaryObj)
	}

	for j, pdfImg := range i.pdfImgs {
		nextID++
		var obj PDFObjData
		obj.objID = nextID
		obj.data = pdfImg.imgObj.GetObjBuff().Bytes()
		i.pdfImgs[j].objID = obj.objID
		//fmt.Printf("---->%d\n", obj.objID)
		newpdf.put(obj)
	}

	err = newpdf.injectImgsToPDF(i.pdfImgs)
	if err != nil {
		return nil, 0, err
	}

	err = newpdf.injectContentToPDF(&i.contenters)
	if err != nil {
		return nil, 0, err
	}

	//set for protection
	if i.protection() != nil {
		max := newpdf.Len()
		x := 0
		for x < max {
			newpdf.objs[x].encrypt(i.protection())
			x++
		}
	}

	return &newpdf, nextID, nil
}

func (i *PDFt) toStream(newpdf *PDFData, lastID int) (*bytes.Buffer, error) {

	//set for protection
	encryptionObjID := -1
	if i.protection() != nil {
		lastID++
		encryptionObjID = lastID
		enObj := i.protection().EncryptionObj()
		err := enObj.Build(lastID)
		if err != nil {
			return nil, err
		}
		buff := enObj.GetObjBuff()
		var enPDFObjData PDFObjData
		enPDFObjData.data = buff.Bytes()
		enPDFObjData.objID = lastID
		newpdf.put(enPDFObjData)
	}

	var buff bytes.Buffer
	buff.WriteString("%PDF-1.7\n\n")
	xrefs := make(map[int]int)
	for _, obj := range newpdf.objs {
		//xrefs = append(xrefs, buff.Len())
		xrefs[obj.objID] = buff.Len()
		buff.WriteString(fmt.Sprintf("\n%d 0 obj\n", obj.objID))
		buff.WriteString(strings.TrimSpace(string(obj.data)))
		buff.WriteString("\nendobj\n")
	}
	i.xref(xrefs, &buff, lastID, newpdf.trailer.rootObjID, encryptionObjID)

	return &buff, nil
}

type xrefrow struct {
	offset int
	gen    string
	flag   string
}

func (i *PDFt) xref(linelens map[int]int, buff *bytes.Buffer, size int, rootID int, encryptionObjID int) {
	xrefbyteoffset := buff.Len()

	//start xref
	buff.WriteString("\nxref\n")
	buff.WriteString(fmt.Sprintf("0 %d\r\n", size))
	var xrefrows []xrefrow
	xrefrows = append(xrefrows, xrefrow{offset: 0, flag: "f", gen: "65535"})
	lastIndexOfF := 0
	j := 1
	for j < size {
		if linelen, ok := linelens[j]; ok {
			xrefrows = append(xrefrows, xrefrow{offset: linelen, flag: "n", gen: "00000"})
		} else {
			xrefrows = append(xrefrows, xrefrow{offset: 0, flag: "f", gen: "65535"})
			offset := len(xrefrows) - 1
			xrefrows[lastIndexOfF].offset = offset
			lastIndexOfF = offset
		}
		j++
	}

	for _, xrefrow := range xrefrows {
		buff.WriteString(i.formatXrefline(xrefrow.offset) + " " + xrefrow.gen + " " + xrefrow.flag + "\n")
	}
	//end xref

	buff.WriteString("trailer\n")
	buff.WriteString("<<\n")
	buff.WriteString(fmt.Sprintf("/Size %d\n", size))
	buff.WriteString(fmt.Sprintf("/Root %d 0 R\n", rootID))
	if i.protection() != nil {
		buff.WriteString(fmt.Sprintf("/Encrypt %d 0 R\n", encryptionObjID))
		buff.WriteString("/ID [()()]\n")
	}
	buff.WriteString(">>\n")

	buff.WriteString("startxref\n")
	buff.WriteString(strconv.Itoa(xrefbyteoffset))
	buff.WriteString("\n%%EOF\n")
}

func (i *PDFt) formatXrefline(n int) string {
	str := strconv.Itoa(n)
	for len(str) < 10 {
		str = "0" + str
	}
	return str
}

//SetProtection set pdf protection
func (i *PDFt) SetProtection(
	permissions int,
	userPass []byte,
	ownerPass []byte,
) error {
	var p gopdf.PDFProtection
	err := p.SetProtection(permissions, userPass, ownerPass)
	if err != nil {
		return err
	}
	i.pdfProtection = &p
	return nil
}
