package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// Article は評価用にエクスポートした記事 1 件。
type Article struct {
	ID      string
	BlogID  string
	URL     string
	Title   string
	Summary string
	Emb     []float32
}

const datasetMagic = "BDEVAL1\n"

// saveDataset は記事一覧を独自バイナリ形式で保存する。
// 形式: magic, 件数(uint32), 次元(uint32), 各記事 [文字列×5 (uint32長+本体), float32×次元]
func saveDataset(path string, arts []Article) error {
	if len(arts) == 0 {
		return errors.New("記事が 0 件です")
	}
	dim := len(arts[0].Emb)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)

	if _, err := w.WriteString(datasetMagic); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, [2]uint32{uint32(len(arts)), uint32(dim)}); err != nil {
		return err
	}
	for _, a := range arts {
		if len(a.Emb) != dim {
			return fmt.Errorf("記事 %s の次元 %d が %d と一致しません", a.ID, len(a.Emb), dim)
		}
		for _, s := range []string{a.ID, a.BlogID, a.URL, a.Title, a.Summary} {
			if err := writeString(w, s); err != nil {
				return err
			}
		}
		if err := binary.Write(w, binary.LittleEndian, a.Emb); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return f.Close()
}

func loadDataset(path string) ([]Article, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)

	magic := make([]byte, len(datasetMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != datasetMagic {
		return nil, fmt.Errorf("%s は評価用データセットではありません", path)
	}
	var hdr [2]uint32
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, err
	}
	n, dim := int(hdr[0]), int(hdr[1])
	arts := make([]Article, n)
	for i := range arts {
		var fields [5]string
		for j := range fields {
			s, err := readString(r)
			if err != nil {
				return nil, fmt.Errorf("記事 %d の読み込み: %w", i, err)
			}
			fields[j] = s
		}
		emb := make([]float32, dim)
		if err := binary.Read(r, binary.LittleEndian, emb); err != nil {
			return nil, fmt.Errorf("記事 %d の埋め込み読み込み: %w", i, err)
		}
		arts[i] = Article{ID: fields[0], BlogID: fields[1], URL: fields[2], Title: fields[3], Summary: fields[4], Emb: emb}
	}
	return arts, nil
}

func writeString(w io.Writer, s string) error {
	if err := binary.Write(w, binary.LittleEndian, uint32(len(s))); err != nil {
		return err
	}
	_, err := io.WriteString(w, s)
	return err
}

func readString(r io.Reader) (string, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return "", err
	}
	if n > math.MaxInt32 {
		return "", errors.New("文字列長が不正です")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}
