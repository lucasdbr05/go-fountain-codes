package chain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/btcsuite/btcd/wire"
)

type BlockSource interface {
	ForEachBlock(fn func(raw []byte, height int) error) error
	BlockCount() (int, error)
}

// BlkDatReader reads Bitcoin blocks from blk*.dat files.
type BlkDatReader struct {
	Dir    string
	Net    wire.BitcoinNet
	Buffer int
	XorKey []byte
}

func NewBlkDatReader(dir string, net wire.BitcoinNet) *BlkDatReader {
	return &BlkDatReader{Dir: dir, Net: net, Buffer: 10}
}

// NewBlkDatReaderAutoXor creates a BlkDatReader and automatically loads xor.dat
// from the blocks directory if it exists, enabling transparent deobfuscation.
func 	NewBlkDatReaderAutoXor(dir string, net wire.BitcoinNet) (*BlkDatReader, error) {
	r := &BlkDatReader{Dir: dir, Net: net, Buffer: 10}
	keyPath := filepath.Join(dir, "xor.dat")
	if key, err := os.ReadFile(keyPath); err == nil && len(key) > 0 {
		r.XorKey = key
	}
	return r, nil
}

func (r *BlkDatReader) xorDecode(buf []byte, fileOffset int64) {
	if len(r.XorKey) == 0 {
		return
	}
	keyLen := int64(len(r.XorKey))
	for i := range buf {
		buf[i] ^= r.XorKey[(fileOffset+int64(i))%keyLen]
	}
}

func (r *BlkDatReader) blkFiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(r.Dir, "blk*.dat"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (r *BlkDatReader) BlockCount() (int, error) {
	files, err := r.blkFiles()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, path := range files {
		n, err := r.countBlocksInFile(path)
		if err != nil {
			return 0, err
		}
		count += n
	}
	if count > r.Buffer {
		count -= r.Buffer
	}
	return count, nil
}

func (r *BlkDatReader) ForEachBlock(fn func(raw []byte, height int) error) error {
	files, err := r.blkFiles()
	if err != nil {
		return err
	}

	// Collect all block offsets first so we can skip the tail buffer.
	type record struct {
		path string
		pos  int64
		size uint32
	}
	var records []record
	for _, path := range files {
		if err := r.walkBlkFile(path, func(pos int64, size uint32) {
			records = append(records, record{path, pos, size})
		}); err != nil {
			return err
		}
	}

	limit := len(records)
	if r.Buffer > 0 && limit > r.Buffer {
		limit -= r.Buffer
	}

	for height, rec := range records[:limit] {
		raw, err := r.readBlockAt(rec.path, rec.pos, rec.size)
		if err != nil {
			return err
		}
		if err := fn(raw, height); err != nil {
			return err
		}
	}
	return nil
}

// ParseBlock deserializes raw bytes as a Bitcoin block using btcd's wire format.
func ParseBlock(raw []byte) (*wire.MsgBlock, error) {
	var msg wire.MsgBlock
	if err := msg.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("wire.MsgBlock.Deserialize: %w", err)
	}
	return &msg, nil
}

// SerializeBlock serializes a wire.MsgBlock back to its canonical byte form.
func SerializeBlock(msg *wire.MsgBlock) ([]byte, error) {
	var buf bytes.Buffer
	if err := msg.Serialize(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *BlkDatReader) countBlocksInFile(path string) (int, error) {
	count := 0
	err := r.walkBlkFile(path, func(_ int64, _ uint32) { count++ })
	return count, err
}

func (r *BlkDatReader) walkBlkFile(path string, fn func(pos int64, size uint32)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	magic := uint32(r.Net)
	var header [8]byte
	for {
		pos, _ := f.Seek(0, io.SeekCurrent)
		if _, err := io.ReadFull(f, header[:]); err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		} else if err != nil {
			return err
		}
		hdr := make([]byte, 8)
		copy(hdr, header[:])
		r.xorDecode(hdr, pos)
		if binary.LittleEndian.Uint32(hdr[:4]) != magic {
			allZero := true
			for _, b := range header[:] {
				if b != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				return nil
			}
			return fmt.Errorf("invalid magic at offset %d in %s", pos, path)
		}
		size := binary.LittleEndian.Uint32(hdr[4:])
		fn(pos+8, size)
		if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
			return err
		}
	}
}

func (r *BlkDatReader) readBlockAt(path string, pos int64, size uint32) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(pos, io.SeekStart); err != nil {
		return nil, err
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(f, raw); err != nil {
		return nil, err
	}
	r.xorDecode(raw, pos)
	return raw, nil
}
