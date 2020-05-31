// Copyright 2019 Tobias Guggenmos
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cache

import (
	"errors"
	"fmt"
	"go/token"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/span"
)

// tokenPositionToProtocolPosition converts a token.Position to a protocol.Position.
func (d *DocumentHandle) tokenPositionToProtocolPosition(pos token.Position) (protocol.Position, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return protocol.Position{}, d.ctx.Err()
	default:
		line := pos.Line
		char := pos.Column

		// Can happen when parsing empty files
		if line < 1 {
			return protocol.Position{
				Line:      0,
				Character: 0,
			}, nil
		}

		// Convert to the Positions as described in the LSP Spec
		lineStart, err := d.lineStartSafe(line)
		if err != nil {
			return protocol.Position{}, err
		}

		offset := int(lineStart) - d.doc.posData.Base() + char - 1
		point := span.NewPoint(line, char, offset)

		char, err = span.ToUTF16Column(point, []byte(d.doc.content))
		// Protocol has zero based positions
		char--
		line--

		if err != nil {
			return protocol.Position{}, err
		}

		return protocol.Position{
			Line:      float64(line),
			Character: float64(char),
		}, nil
	}
}

// PosToProtocolPosition converts a token.Pos to a protocol.Position.
func (d *DocumentHandle) PosToProtocolPosition(pos token.Pos) (protocol.Position, error) {
	ret, err := d.tokenPositionToProtocolPosition(d.doc.posData.Position(pos))
	return ret, err
}

// protocolPositionToTokenPos converts a token.Pos to a protocol.Position.
func (d *DocumentHandle) protocolPositionToTokenPos(pos protocol.Position) (token.Pos, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()

	select {
	case <-d.ctx.Done():
		return 0, d.ctx.Err()
	default:
		// protocol.Position is 0 based.
		line := int(pos.Line) + 1
		char := int(pos.Character)

		lineStart, err := d.lineStartSafe(line)
		if err != nil {
			return token.NoPos, err
		}

		offset := int(lineStart) - d.doc.posData.Base()
		point := span.NewPoint(line, 1, offset)

		point, err = span.FromUTF16Column(point, char, []byte(d.doc.content))
		if err != nil {
			return token.NoPos, err
		}

		char = point.Column()

		return lineStart + token.Pos(char), nil
	}
}

// yamlPositionToTokenPos converts a position of the format used by the yaml parser to a token.Pos.
func (d *DocumentHandle) yamlPositionToTokenPos(line int, column int, lineOffset int) (token.Pos, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()
	select {
	case <-d.ctx.Done():
		return token.NoPos, d.ctx.Err()
	default:
		if column < 1 {
			return 0, errors.New("invalid position")
		}

		lineStart, err := d.lineStartSafe(line + lineOffset)
		if err != nil {
			return token.NoPos, err
		}

		return lineStart + token.Pos(column-1), nil
	}
}

// endOfLine returns the end of the Line of the given protocol.Position.
func endOfLine(p protocol.Position) protocol.Position {
	return protocol.Position{
		Line:      p.Line + 1,
		Character: 0,
	}
}

// lineStartSafe is a wrapper around token.File.LineStart() that does not panic on Error.
func (d *DocumentHandle) lineStartSafe(line int) (pos token.Pos, err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("LineStart panic: %v", r)
			}
		}
	}()

	return d.doc.posData.LineStart(line), nil
}

// tokenPosToTokenPosition converts a token.Pos to a token.Position.
func (d *DocumentHandle) tokenPosToTokenPosition(pos token.Pos) (token.Position, error) {
	d.doc.mu.RLock()
	defer d.doc.mu.RUnlock()
	select {
	case <-d.ctx.Done():
		return token.Position{}, d.ctx.Err()
	default:
		return d.doc.posData.Position(pos), nil
	}
}
