package ares

import (
	"bytes"
	"io"
)

func NewChangeStream(r io.ReadCloser) io.ReadCloser {
	return &ChangeStream{r, []byte{}}
}

type ChangeStream struct {
	io.ReadCloser

	// being used for temporarily rest, when being replaced with longer
	overflow []byte
}

func (cs *ChangeStream) Read(p []byte) (n int, err error) {
	copy(p, cs.overflow)

	n, err = cs.ReadCloser.Read(p[len(cs.overflow):])
	if err == io.EOF {
	} else if err != nil {
		return n, err
	}

	cs.overflow = []byte{}

	needle := []byte("Politie")

	repl := []byte("eitiloP")

	// currently we are assuming:
	for i := 0; i < n-len(needle); i++ {
		if bytes.Compare(p[i:i+len(needle)], needle) != 0 {
			continue
		}

		newIndex := i

		// take care of longer, sizes, put in rest buffer.
		for j := 0; j < len(repl); j++ {
			p[newIndex] = repl[j]
			newIndex++
		}

		oldIndex := i + len(needle)
		for oldIndex < n {
			p[newIndex] = p[oldIndex]
			oldIndex++
			newIndex++
		}

		n = newIndex
	}

	return n, err
}

func (cs *ChangeStream) Close() error {
	return cs.ReadCloser.Close()
}
