package plyfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unsafe"
)

var propertySizes = map[string]int{
	"char":   1,
	"uchar":  1,
	"short":  2,
	"ushort": 2,
	"int":    4,
	"uint":   4,
	"float":  4,
	"double": 8,
}

const regExpFormat = "^format (ascii|binary_little_endian).*"
const regExpComment = "^comment (.*)"
const regExpElement = "^element (\\w*) (\\d*)"
const regExpProperty = "^property (char|uchar|short|ushort|int|uint|float|double) (\\w*)"
const headerEnd = "end_header"

type ElementReader struct {
	pos     int64
	offset  int64
	file    *os.File
	element *element
}

type element struct {
	Name       string
	Count      int64
	Properties []*property
}

type property struct {
	Type string
	Name string
	Size int
}

type header struct {
	Format   string
	Comment  string
	offset   int64
	Elements []*element
}

type PlyFile struct {
	file      *os.File
	headerStr string
	header    *header
}

func memcpy(bits []byte, dest unsafe.Pointer) {
	copy(unsafe.Slice((*byte)(unsafe.Pointer(dest)), len(bits)), bits)
}

func propertySize(t string) int {
	if size, ok := propertySizes[t]; ok {
		return size
	}

	return 0
}

func (e element) PointByteSize() int {
	size := 0
	for _, prop := range e.Properties {
		size += prop.Size
	}
	return size
}

func Open(name string) (*PlyFile, error) {
	file, err := os.Open(name)

	if err != nil {
		return nil, err
	}

	ply := &PlyFile{
		file: file,
		header: &header{
			offset: 0,
		},
	}

	if err = ply.readHeader(); err != nil {
		file.Close()
		return nil, err
	}

	ply.parseHeader()

	if ply.header.Format != "binary_little_endian" {
		file.Close()
		return nil, errors.New("binary_little_endian support only")
	}

	return ply, nil
}

func (f *PlyFile) Close() error {
	f.header.offset = 0
	f.headerStr = ""

	return f.file.Close()
}

func (f *PlyFile) readHeader() error {
	f.headerStr = ""
	f.header.offset = 0
	isHeaderEnd := false
	buf := make([]byte, 100)

	for isHeaderEnd != true {
		n, err := f.file.Read(buf)
		if err != nil {
			return errors.New("failed read header")
		}

		f.headerStr = f.headerStr + string(buf[:n])
		if pos := strings.Index(f.headerStr, "end_header"); pos != -1 {
			isHeaderEnd = true
			f.header.offset = int64(pos + len("end_header") + 1)
			f.headerStr = f.headerStr[:f.header.offset]
		}
	}

	if f.headerStr == "" {
		return errors.New("invalid ply file")
	}

	return nil
}

func (f *PlyFile) parseHeader() {
	headerLines := strings.Split(f.headerStr, "\n")

	rFormat, _ := regexp.Compile(regExpFormat)
	rComment, _ := regexp.Compile(regExpComment)
	rElement, _ := regexp.Compile(regExpElement)
	rProperty, _ := regexp.Compile(regExpProperty)

	var currElement *element = nil

	for _, line := range headerLines {
		if res := rFormat.FindAllStringSubmatch(line, -1); len(res) != 0 {
			f.header.Format = res[0][1]
			continue
		}

		if res := rComment.FindAllStringSubmatch(line, -1); len(res) != 0 {
			f.header.Comment = res[0][1]
			continue
		}

		if res := rElement.FindAllStringSubmatch(line, -1); len(res) != 0 {
			if currElement != nil {
				f.header.Elements = append(f.header.Elements, currElement)
			}

			count, _ := strconv.Atoi(res[0][2])
			currElement = &element{
				Name:  res[0][1],
				Count: int64(count),
			}
			continue
		}

		if res := rProperty.FindAllStringSubmatch(line, -1); len(res) != 0 {
			currElement.Properties = append(
				currElement.Properties,
				&property{
					Type: res[0][1],
					Name: res[0][2],
					Size: propertySize(res[0][1]),
				},
			)
		}

		if line == headerEnd {
			f.header.Elements = append(f.header.Elements, currElement)
		}
	}
}

func (f *PlyFile) Has(name string) bool {
	for i := 0; i < len(f.header.Elements); i++ {
		if f.header.Elements[i].Name == name {
			return true
		}
	}

	return false
}

func (f *PlyFile) getElement(name string) *element {
	for _, e := range f.header.Elements {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func (f *PlyFile) getElementOffset(name string) int64 {
	if !f.Has(name) {
		return -1
	}

	var offset int64 = 0

	for _, e := range f.header.Elements {
		if e.Name != name {
			offset += e.Count * int64(e.PointByteSize())
		} else {
			break
		}
	}

	return offset
}

func (f *PlyFile) GetElementReader(name string) (*ElementReader, error) {
	if !f.Has(name) {
		return nil, errors.New(fmt.Sprintf("unknown element '%s'", name))
	}

	return &ElementReader{
		file:    f.file,
		offset:  f.header.offset + f.getElementOffset(name),
		pos:     0,
		element: f.getElement(name),
	}, nil
}

func (r *ElementReader) Seek(pos int64) error {
	if pos < 0 || pos > r.element.Count {
		return errors.New(fmt.Sprintf("can't offset on %d position", pos))
	}

	r.pos = pos
	return nil
}

func (r *ElementReader) Reset() error {
	return r.Seek(0)
}

func (r *ElementReader) ReadNext(pointer interface{}) (int64, error) {
	_, err := r.file.Seek(r.offset+(r.pos*int64(r.element.PointByteSize())), 0)

	if err != nil {
		return -1, err
	}

	buf := make([]byte, r.element.PointByteSize())

	if r.pos >= r.element.Count {
		return -1, io.EOF
	}

	_, err = r.file.Read(buf)

	if err == io.EOF {
		return -1, err
	}

	offset := 0

	for i := 0; i < len(r.element.Properties); i++ {
		prop := r.element.Properties[i]

		t := reflect.TypeOf(pointer).Elem()
		v := reflect.Indirect(reflect.ValueOf(pointer))
		if t.Kind() == reflect.Struct {
			for i := 0; i < v.NumField(); i++ {
				if t.Field(i).Tag.Get("ply") == prop.Name {
					switch prop.Type {
					case "char", "uchar":
						v := byte(0)
						memcpy(buf[offset:offset+prop.Size], unsafe.Pointer(&v))
						reflect.ValueOf(pointer).Elem().Field(i).Set(reflect.ValueOf(v))
					case "short":
						v := int16(0)
						memcpy(buf[offset:offset+prop.Size], unsafe.Pointer(&v))
						reflect.ValueOf(pointer).Elem().Field(i).Set(reflect.ValueOf(v))
					case "ushort":
						v := uint16(0)
						memcpy(buf[offset:offset+prop.Size], unsafe.Pointer(&v))
						reflect.ValueOf(pointer).Elem().Field(i).Set(reflect.ValueOf(v))
					case "int":
						v := int32(0)
						memcpy(buf[offset:offset+prop.Size], unsafe.Pointer(&v))
						reflect.ValueOf(pointer).Elem().Field(i).Set(reflect.ValueOf(v))
					case "uint":
						v := uint32(0)
						memcpy(buf[offset:offset+prop.Size], unsafe.Pointer(&v))
						reflect.ValueOf(pointer).Elem().Field(i).Set(reflect.ValueOf(v))
					case "float":
						v := float32(0)
						memcpy(buf[offset:offset+prop.Size], unsafe.Pointer(&v))
						reflect.ValueOf(pointer).Elem().Field(i).Set(reflect.ValueOf(v))
					case "double":
						v := float64(0)
						memcpy(buf[offset:offset+prop.Size], unsafe.Pointer(&v))
						reflect.ValueOf(pointer).Elem().Field(i).Set(reflect.ValueOf(v))
					}
				}
			}
		}

		offset += prop.Size
	}

	r.pos++

	return r.pos, nil
}

func (r *ElementReader) ReadAt(pos int64, pointer interface{}) error {
	currPos := r.pos
	if err := r.Seek(pos); err != nil {
		return err
	}

	if _, err := r.ReadNext(pointer); err != nil {
		return err
	}
	r.pos = currPos
	return nil
}

func (r *ElementReader) ReadFirst(pointer interface{}) error {
	return r.ReadAt(0, pointer)
}

func (r *ElementReader) CurrentPos() int64 {
	return r.pos
}

func (r *ElementReader) Count() int64 {
	return r.element.Count
}

type Point struct {
	X float64 `ply:"x"`
	Y float64 `ply:"y"`
	Z float64 `ply:"z"`
	R byte    `ply:"red"`
	G byte    `ply:"green"`
	B byte    `ply:"blue"`
}

func (p Point) String() string {
	return fmt.Sprintf(
		"{x: %.16f, y: %.16f, z: %.16f, r: %d, g: %d, b: %d}",
		p.X, p.Y, p.Z, p.R, p.G, p.B,
	)
}
