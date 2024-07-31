package relabeler

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"

	"github.com/go-playground/validator/v10"
)

// CurrentState - current state data.
type CurrentState struct {
	shardsNumberPower *uint8
	limits            *Limits
	validate          *validator.Validate
	dir               string
	fileName          string
}

// NewCurrentState - init new CurrentState.
func NewCurrentState(dir string) *CurrentState {
	return &CurrentState{
		dir:      dir,
		fileName: "delivery_state.db",
		validate: validator.New(),
	}
}

// ShardsNumberPower - return if exist value ShardsNumberPower or default.
func (cs *CurrentState) ShardsNumberPower() uint8 {
	if cs.shardsNumberPower == nil {
		return DefaultShardsNumberPower
	}
	return *cs.shardsNumberPower
}

// Limits - return current all limits.
func (cs *CurrentState) Limits() *Limits {
	if cs.limits == nil {
		return DefaultLimits()
	}
	if err := cs.validate.Struct(cs.limits); err != nil {
		return DefaultLimits()
	}
	return cs.limits
}

// Block - return current BlockLimits.
func (cs *CurrentState) Block() BlockLimits {
	if cs.limits == nil {
		return DefaultBlockLimits()
	}
	if err := cs.validate.Struct(cs.limits.Block); err != nil {
		return DefaultBlockLimits()
	}
	return cs.limits.Block
}

// Read - read current state data from file.
func (cs *CurrentState) Read() error {
	b, err := os.ReadFile(filepath.Join(cs.dir, cs.fileName))
	if err != nil {
		return fmt.Errorf("read file %s: %w", cs.fileName, err)
	}

	if err = cs.unmarshal(b); err != nil {
		cs.limits = nil
		cs.shardsNumberPower = nil
		return fmt.Errorf("unmarshal: %w", err)
	}

	return nil
}

// unmarshal - decoding from byte.
//
//revive:disable-next-line:function-length long but readable
//revive:disable-next-line:cyclomatic  but readable
func (cs *CurrentState) unmarshal(data []byte) error {
	var offset int

	mb, n := binary.Uvarint(data[offset:])
	if byte(mb) != magicByte {
		return fmt.Errorf("%w: file dont have magic byte: %d", ErrCorruptedFile, mb)
	}
	offset += n

	// read shards number power with checksum
	chksm, n := binary.Uvarint(data[offset:])
	offset += n
	length, n := binary.Uvarint(data[offset:])
	offset += n
	if uint32(chksm) != crc32.ChecksumIEEE(data[offset:offset+int(length)]) {
		return fmt.Errorf("%w: check sum not equal for shards number power", ErrCorruptedFile)
	}
	usnp, n := binary.Uvarint(data[offset : offset+int(length)])
	snp := uint8(usnp)
	cs.shardsNumberPower = &snp
	offset += n

	if cs.limits == nil {
		cs.limits = new(Limits)
	}

	// read open head limits with checksum
	chksm, n = binary.Uvarint(data[offset:])
	offset += n
	length, n = binary.Uvarint(data[offset:])
	offset += n
	if uint32(chksm) != crc32.ChecksumIEEE(data[offset:offset+int(length)]) {
		return fmt.Errorf("%w: check sum not equal for open head limits", ErrCorruptedFile)
	}
	if err := cs.limits.OpenHead.UnmarshalBinary(data[offset : offset+int(length)]); err != nil {
		return err
	}
	offset += int(length)

	// read block limits with checksum
	chksm, n = binary.Uvarint(data[offset:])
	offset += n
	length, n = binary.Uvarint(data[offset:])
	offset += n
	if uint32(chksm) != crc32.ChecksumIEEE(data[offset:offset+int(length)]) {
		return fmt.Errorf("%w: check sum not equal for block limits", ErrCorruptedFile)
	}
	if err := cs.limits.Block.UnmarshalBinary(data[offset : offset+int(length)]); err != nil {
		return err
	}
	offset += int(length)

	// read hashdex limits with checksum
	chksm, n = binary.Uvarint(data[offset:])
	offset += n
	length, n = binary.Uvarint(data[offset:])
	offset += n
	if uint32(chksm) != crc32.ChecksumIEEE(data[offset:offset+int(length)]) {
		return fmt.Errorf("%w: check sum not equal for hashdex limits", ErrCorruptedFile)
	}
	if err := cs.limits.Hashdex.UnmarshalBinary(data[offset : offset+int(length)]); err != nil {
		return err
	}
	offset += int(length)

	// read checksum file
	chksm, n = binary.Uvarint(data[offset:])
	if uint32(chksm) != crc32.ChecksumIEEE(data[:len(data)-n]) {
		return fmt.Errorf("%w: check sum not equal", ErrCorruptedFile)
	}
	return nil
}

// Write - write current state data in file.
func (cs *CurrentState) Write(snp uint8, limits *Limits) error {
	cs.shardsNumberPower = &snp
	cs.limits = limits
	out, err := cs.marshal()
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	//revive:disable-next-line:add-constant file permissions simple readable as octa-number
	if err = os.MkdirAll(cs.dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(cs.dir), err)
	}

	//revive:disable-next-line:add-constant file permissions simple readable as octa-number
	if err = os.WriteFile(filepath.Join(cs.dir, cs.fileName), out, 0o600); err != nil {
		return fmt.Errorf("write file %s: %w", cs.fileName, err)
	}

	return nil
}

// marshal - encoding to byte.
func (cs *CurrentState) marshal() ([]byte, error) {
	snpb := binary.AppendUvarint([]byte{}, uint64(cs.ShardsNumberPower()))

	ohlb, err := cs.limits.OpenHead.MarshalBinary()
	if err != nil {
		return nil, err
	}

	blb, err := cs.limits.Block.MarshalBinary()
	if err != nil {
		return nil, err
	}

	hlb, err := cs.limits.Hashdex.MarshalBinary()
	if err != nil {
		return nil, err
	}

	//revive:disable-next-line:add-constant 2 magicByte + 1 (shardsNumberPower) + max size len 1 byte + 5*4(Checksum)
	bufSize := 32 + len(ohlb) + len(blb) + len(hlb)
	buf := make([]byte, 0, bufSize)
	buf = binary.AppendUvarint(buf, uint64(magicByte))

	// write shards number power with checksum
	buf = binary.AppendUvarint(buf, uint64(crc32.ChecksumIEEE(snpb)))
	buf = binary.AppendUvarint(buf, uint64(len(snpb)))
	buf = append(buf, snpb...)

	// write open head limits with checksum
	buf = binary.AppendUvarint(buf, uint64(crc32.ChecksumIEEE(ohlb)))
	buf = binary.AppendUvarint(buf, uint64(len(ohlb)))
	buf = append(buf, ohlb...)

	// write block limits with checksum
	buf = binary.AppendUvarint(buf, uint64(crc32.ChecksumIEEE(blb)))
	buf = binary.AppendUvarint(buf, uint64(len(blb)))
	buf = append(buf, blb...)

	// write hashdex limits with checksum
	buf = binary.AppendUvarint(buf, uint64(crc32.ChecksumIEEE(hlb)))
	buf = binary.AppendUvarint(buf, uint64(len(hlb)))
	buf = append(buf, hlb...)

	buf = binary.AppendUvarint(buf, uint64(crc32.ChecksumIEEE(buf)))

	return buf, nil
}
