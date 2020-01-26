/*
Copyright 2015 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Package bttest contains test helpers for working with the bigtable package.

To use a Server, create it, and then connect to it with no security:
(The project/instance values are ignored.)
	srv, err := bttest.NewServer("localhost:0")
	...
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	...
	client, err := bigtable.NewClient(ctx, proj, instance,
	        option.WithGRPCConn(conn))
	...
*/
package bttest // import "cloud.google.com/go/bigtable/bttest"

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	emptypb "github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/btree"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/genproto/googleapis/longrunning"
	statpb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// MilliSeconds field of the minimum valid Timestamp.
	minValidMilliSeconds = 0

	// MilliSeconds field of the max valid Timestamp.
	maxValidMilliSeconds = int64(time.Millisecond) * 253402300800
)

var (
	validLabelTransformer = regexp.MustCompile(`[a-z0-9\-]{1,15}`)
)

// Server is an in-memory Cloud Bigtable fake.
// It is unauthenticated, and only a rough approximation.
type Server struct {
	Addr string

	l   net.Listener
	srv *grpc.Server
	s   *server
}

// server is the real implementation of the fake.
// It is a separate and unexported type so the API won't be cluttered with
// methods that are only relevant to the fake's implementation.
type server struct {
	mu     sync.Mutex
	tables map[string]*table // keyed by fully qualified name
	gcc    chan int          // set when gcloop starts, closed when server shuts down

	// Any unimplemented methods will cause a panic.
	btapb.BigtableTableAdminServer
	btpb.BigtableServer
}

// NewServer creates a new Server.
// The Server will be listening for gRPC connections, without TLS,
// on the provided address. The resolved address is named by the Addr field.
func NewServer(laddr string, opt ...grpc.ServerOption) (*Server, error) {
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}

	s := &Server{
		Addr: l.Addr().String(),
		l:    l,
		srv:  grpc.NewServer(opt...),
		s: &server{
			tables: make(map[string]*table),
		},
	}
	btapb.RegisterBigtableTableAdminServer(s.srv, s.s)
	btpb.RegisterBigtableServer(s.srv, s.s)

	go s.srv.Serve(s.l)

	return s, nil
}

// Close shuts down the server.
func (s *Server) Close() {
	s.s.mu.Lock()
	if s.s.gcc != nil {
		close(s.s.gcc)
	}
	s.s.mu.Unlock()

	s.srv.Stop()
	s.l.Close()
}

func (s *server) CreateTable(ctx context.Context, req *btapb.CreateTableRequest) (*btapb.Table, error) {
	tbl := req.Parent + "/tables/" + req.TableId

	s.mu.Lock()
	if _, ok := s.tables[tbl]; ok {
		s.mu.Unlock()
		return nil, status.Errorf(codes.AlreadyExists, "table %q already exists", tbl)
	}
	s.tables[tbl] = newTable(req)
	s.mu.Unlock()

	return &btapb.Table{Name: tbl}, nil
}

func (s *server) CreateTableFromSnapshot(context.Context, *btapb.CreateTableFromSnapshotRequest) (*longrunning.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "the emulator does not currently support snapshots")
}

func (s *server) ListTables(ctx context.Context, req *btapb.ListTablesRequest) (*btapb.ListTablesResponse, error) {
	res := &btapb.ListTablesResponse{}
	prefix := req.Parent + "/tables/"

	s.mu.Lock()
	for tbl := range s.tables {
		if strings.HasPrefix(tbl, prefix) {
			res.Tables = append(res.Tables, &btapb.Table{Name: tbl})
		}
	}
	s.mu.Unlock()

	return res, nil
}

func (s *server) GetTable(ctx context.Context, req *btapb.GetTableRequest) (*btapb.Table, error) {
	tbl := req.Name

	s.mu.Lock()
	tblIns, ok := s.tables[tbl]
	s.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", tbl)
	}

	return &btapb.Table{
		Name:           tbl,
		ColumnFamilies: toColumnFamilies(tblIns.columnFamilies()),
	}, nil
}

func (s *server) DeleteTable(ctx context.Context, req *btapb.DeleteTableRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tables[req.Name]; !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.Name)
	}
	delete(s.tables, req.Name)
	return &emptypb.Empty{}, nil
}

func (s *server) ModifyColumnFamilies(ctx context.Context, req *btapb.ModifyColumnFamiliesRequest) (*btapb.Table, error) {
	s.mu.Lock()
	tbl, ok := s.tables[req.Name]
	s.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.Name)
	}

	tbl.mu.Lock()
	defer tbl.mu.Unlock()

	for _, mod := range req.Modifications {
		if create := mod.GetCreate(); create != nil {
			if _, ok := tbl.families[mod.Id]; ok {
				return nil, status.Errorf(codes.AlreadyExists, "family %q already exists", mod.Id)
			}
			newcf := &columnFamily{
				name:   req.Name + "/columnFamilies/" + mod.Id,
				order:  tbl.counter,
				gcRule: create.GcRule,
			}
			tbl.counter++
			tbl.families[mod.Id] = newcf
		} else if mod.GetDrop() {
			if _, ok := tbl.families[mod.Id]; !ok {
				return nil, fmt.Errorf("can't delete unknown family %q", mod.Id)
			}
			delete(tbl.families, mod.Id)
		} else if modify := mod.GetUpdate(); modify != nil {
			if _, ok := tbl.families[mod.Id]; !ok {
				return nil, fmt.Errorf("no such family %q", mod.Id)
			}
			newcf := &columnFamily{
				name:   req.Name + "/columnFamilies/" + mod.Id,
				gcRule: modify.GcRule,
			}
			// assume that we ALWAYS want to replace by the new setting
			// we may need partial update through
			tbl.families[mod.Id] = newcf
		}
	}

	s.needGC()
	return &btapb.Table{
		Name:           req.Name,
		ColumnFamilies: toColumnFamilies(tbl.families),
		Granularity:    btapb.Table_TimestampGranularity(btapb.Table_MILLIS),
	}, nil
}

func (s *server) DropRowRange(ctx context.Context, req *btapb.DropRowRangeRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tbl, ok := s.tables[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.Name)
	}

	if req.GetDeleteAllDataFromTable() {
		tbl.rows = btree.New(btreeDegree)
	} else {
		// Delete rows by prefix.
		prefixBytes := req.GetRowKeyPrefix()
		if prefixBytes == nil {
			return nil, fmt.Errorf("missing row key prefix")
		}
		prefix := string(prefixBytes)

		// The BTree does not specify what happens if rows are deleted during
		// iteration, and it provides no "delete range" method.
		// So we collect the rows first, then delete them one by one.
		var rowsToDelete []*row
		tbl.rows.AscendGreaterOrEqual(btreeKey(prefix), func(i btree.Item) bool {
			r := i.(*row)
			if strings.HasPrefix(r.key, prefix) {
				rowsToDelete = append(rowsToDelete, r)
				return true
			}
			return false // stop iteration
		})
		for _, r := range rowsToDelete {
			tbl.rows.Delete(r)
		}
	}
	return &emptypb.Empty{}, nil
}

func (s *server) GenerateConsistencyToken(ctx context.Context, req *btapb.GenerateConsistencyTokenRequest) (*btapb.GenerateConsistencyTokenResponse, error) {
	// Check that the table exists.
	_, ok := s.tables[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.Name)
	}

	return &btapb.GenerateConsistencyTokenResponse{
		ConsistencyToken: "TokenFor-" + req.Name,
	}, nil
}

func (s *server) CheckConsistency(ctx context.Context, req *btapb.CheckConsistencyRequest) (*btapb.CheckConsistencyResponse, error) {
	// Check that the table exists.
	_, ok := s.tables[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.Name)
	}

	// Check this is the right token.
	if req.ConsistencyToken != "TokenFor-"+req.Name {
		return nil, status.Errorf(codes.InvalidArgument, "token %q not valid", req.ConsistencyToken)
	}

	// Single cluster instances are always consistent.
	return &btapb.CheckConsistencyResponse{
		Consistent: true,
	}, nil
}

func (s *server) SnapshotTable(context.Context, *btapb.SnapshotTableRequest) (*longrunning.Operation, error) {
	return nil, status.Errorf(codes.Unimplemented, "the emulator does not currently support snapshots")
}

func (s *server) GetSnapshot(context.Context, *btapb.GetSnapshotRequest) (*btapb.Snapshot, error) {
	return nil, status.Errorf(codes.Unimplemented, "the emulator does not currently support snapshots")
}
func (s *server) ListSnapshots(context.Context, *btapb.ListSnapshotsRequest) (*btapb.ListSnapshotsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "the emulator does not currently support snapshots")
}
func (s *server) DeleteSnapshot(context.Context, *btapb.DeleteSnapshotRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "the emulator does not currently support snapshots")
}

func (s *server) ReadRows(req *btpb.ReadRowsRequest, stream btpb.Bigtable_ReadRowsServer) error {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return status.Errorf(codes.NotFound, "table %q not found", req.TableName)
	}

	// Rows to read can be specified by a set of row keys and/or a set of row ranges.
	// Output is a stream of sorted, de-duped rows.
	tbl.mu.RLock()
	rowSet := make(map[string]*row)

	addRow := func(i btree.Item) bool {
		r := i.(*row)
		rowSet[r.key] = r
		return true
	}

	if req.Rows != nil &&
		len(req.Rows.RowKeys)+len(req.Rows.RowRanges) > 0 {
		// Add the explicitly given keys
		for _, key := range req.Rows.RowKeys {
			k := string(key)
			if i := tbl.rows.Get(btreeKey(k)); i != nil {
				addRow(i)
			}
		}

		// Add keys from row ranges
		for _, rr := range req.Rows.RowRanges {
			var start, end string
			switch sk := rr.StartKey.(type) {
			case *btpb.RowRange_StartKeyClosed:
				start = string(sk.StartKeyClosed)
			case *btpb.RowRange_StartKeyOpen:
				start = string(sk.StartKeyOpen) + "\x00"
			}
			switch ek := rr.EndKey.(type) {
			case *btpb.RowRange_EndKeyClosed:
				end = string(ek.EndKeyClosed) + "\x00"
			case *btpb.RowRange_EndKeyOpen:
				end = string(ek.EndKeyOpen)
			}
			switch {
			case start == "" && end == "":
				tbl.rows.Ascend(addRow) // all rows
			case start == "":
				tbl.rows.AscendLessThan(btreeKey(end), addRow)
			case end == "":
				tbl.rows.AscendGreaterOrEqual(btreeKey(start), addRow)
			default:
				tbl.rows.AscendRange(btreeKey(start), btreeKey(end), addRow)
			}
		}
	} else {
		// Read all rows
		tbl.rows.Ascend(addRow)
	}
	tbl.mu.RUnlock()

	rows := make([]*row, 0, len(rowSet))
	for _, r := range rowSet {
		r.mu.Lock()
		fams := len(r.families)
		r.mu.Unlock()

		if fams != 0 {
			rows = append(rows, r)
		}
	}
	sort.Sort(byRowKey(rows))

	limit := int(req.RowsLimit)
	count := 0
	for _, r := range rows {
		if limit > 0 && count >= limit {
			return nil
		}
		streamed, err := streamRow(stream, r, req.Filter)
		if err != nil {
			return err
		}
		if streamed {
			count++
		}
	}
	return nil
}

// streamRow filters the given row and sends it via the given stream.
// Returns true if at least one cell matched the filter and was streamed, false otherwise.
func streamRow(stream btpb.Bigtable_ReadRowsServer, r *row, f *btpb.RowFilter) (bool, error) {
	r.mu.Lock()
	nr := r.copy()
	r.mu.Unlock()
	r = nr

	match, err := filterRow(f, r)
	if err != nil {
		return false, err
	}
	if !match {
		return false, nil
	}

	rrr := &btpb.ReadRowsResponse{}
	families := r.sortedFamilies()
	for _, fam := range families {
		for _, colName := range fam.colNames {
			cells := fam.cells[colName]
			if len(cells) == 0 {
				continue
			}
			for _, cell := range cells {
				rrr.Chunks = append(rrr.Chunks, &btpb.ReadRowsResponse_CellChunk{
					RowKey:          []byte(r.key),
					FamilyName:      &wrappers.StringValue{Value: fam.name},
					Qualifier:       &wrappers.BytesValue{Value: []byte(colName)},
					TimestampMicros: cell.ts,
					Value:           cell.value,
					Labels:          cell.labels,
				})
			}
		}
	}
	// We can't have a cell with just COMMIT set, which would imply a new empty cell.
	// So modify the last cell to have the COMMIT flag set.
	if len(rrr.Chunks) > 0 {
		rrr.Chunks[len(rrr.Chunks)-1].RowStatus = &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true}
	}

	return true, stream.Send(rrr)
}

// filterRow modifies a row with the given filter. Returns true if at least one cell from the row matches,
// false otherwise. If a filter is invalid, filterRow returns false and an error.
func filterRow(f *btpb.RowFilter, r *row) (bool, error) {
	if f == nil {
		return true, nil
	}
	// Handle filters that apply beyond just including/excluding cells.
	switch f := f.Filter.(type) {
	case *btpb.RowFilter_BlockAllFilter:
		return !f.BlockAllFilter, nil
	case *btpb.RowFilter_PassAllFilter:
		return f.PassAllFilter, nil
	case *btpb.RowFilter_Chain_:
		for _, sub := range f.Chain.Filters {
			match, err := filterRow(sub, r)
			if err != nil {
				return false, err
			}
			if !match {
				return false, nil
			}
		}
		return true, nil
	case *btpb.RowFilter_Interleave_:
		srs := make([]*row, 0, len(f.Interleave.Filters))
		for _, sub := range f.Interleave.Filters {
			sr := r.copy()
			match, err := filterRow(sub, sr)
			if err != nil {
				return false, err
			}
			if match {
				srs = append(srs, sr)
			}
		}
		// merge
		// TODO(dsymonds): is this correct?
		r.families = make(map[string]*family)
		for _, sr := range srs {
			for _, fam := range sr.families {
				f := r.getOrCreateFamily(fam.name, fam.order)
				for colName, cs := range fam.cells {
					f.cells[colName] = append(f.cellsByColumn(colName), cs...)
				}
			}
		}
		var count int
		for _, fam := range r.families {
			for _, cs := range fam.cells {
				sort.Sort(byDescTS(cs))
				count += len(cs)
			}
		}
		return count > 0, nil
	case *btpb.RowFilter_CellsPerColumnLimitFilter:
		lim := int(f.CellsPerColumnLimitFilter)
		for _, fam := range r.families {
			for col, cs := range fam.cells {
				if len(cs) > lim {
					fam.cells[col] = cs[:lim]
				}
			}
		}
		return true, nil
	case *btpb.RowFilter_Condition_:
		match, err := filterRow(f.Condition.PredicateFilter, r.copy())
		if err != nil {
			return false, err
		}
		if match {
			if f.Condition.TrueFilter == nil {
				return false, nil
			}
			return filterRow(f.Condition.TrueFilter, r)
		}
		if f.Condition.FalseFilter == nil {
			return false, nil
		}
		return filterRow(f.Condition.FalseFilter, r)
	case *btpb.RowFilter_RowKeyRegexFilter:
		rx, err := newRegexp(f.RowKeyRegexFilter)
		if err != nil {
			return false, status.Errorf(codes.InvalidArgument, "Error in field 'rowkey_regex_filter' : %v", err)
		}
		if !rx.MatchString(r.key) {
			return false, nil
		}
	case *btpb.RowFilter_CellsPerRowLimitFilter:
		// Grab the first n cells in the row.
		lim := int(f.CellsPerRowLimitFilter)
		for _, fam := range r.families {
			for _, col := range fam.colNames {
				cs := fam.cells[col]
				if len(cs) > lim {
					fam.cells[col] = cs[:lim]
					lim = 0
				} else {
					lim -= len(cs)
				}
			}
		}
		return true, nil
	case *btpb.RowFilter_CellsPerRowOffsetFilter:
		// Skip the first n cells in the row.
		offset := int(f.CellsPerRowOffsetFilter)
		for _, fam := range r.families {
			for _, col := range fam.colNames {
				cs := fam.cells[col]
				if len(cs) > offset {
					fam.cells[col] = cs[offset:]
					offset = 0
					return true, nil
				}
				fam.cells[col] = cs[:0]
				offset -= len(cs)
			}
		}
		return true, nil
	case *btpb.RowFilter_RowSampleFilter:
		// The row sample filter "matches all cells from a row with probability
		// p, and matches no cells from the row with probability 1-p."
		// See https://github.com/googleapis/googleapis/blob/master/google/bigtable/v2/data.proto
		if f.RowSampleFilter <= 0.0 || f.RowSampleFilter >= 1.0 {
			return false, status.Error(codes.InvalidArgument, "row_sample_filter argument must be between 0.0 and 1.0")
		}
		return randFloat() < f.RowSampleFilter, nil
	}

	// Any other case, operate on a per-cell basis.
	cellCount := 0
	for _, fam := range r.families {
		for colName, cs := range fam.cells {
			filtered, err := filterCells(f, fam.name, colName, cs)
			if err != nil {
				return false, err
			}
			fam.cells[colName] = filtered
			cellCount += len(fam.cells[colName])
		}
	}
	return cellCount > 0, nil
}

var randFloat = rand.Float64

func filterCells(f *btpb.RowFilter, fam, col string, cs []cell) ([]cell, error) {
	var ret []cell
	for _, cell := range cs {
		include, err := includeCell(f, fam, col, cell)
		if err != nil {
			return nil, err
		}
		if include {
			cell, err = modifyCell(f, cell)
			if err != nil {
				return nil, err
			}
			ret = append(ret, cell)
		}
	}
	return ret, nil
}

func modifyCell(f *btpb.RowFilter, c cell) (cell, error) {
	if f == nil {
		return c, nil
	}
	// Consider filters that may modify the cell contents
	switch filter := f.Filter.(type) {
	case *btpb.RowFilter_StripValueTransformer:
		return cell{ts: c.ts}, nil
	case *btpb.RowFilter_ApplyLabelTransformer:
		if !validLabelTransformer.MatchString(filter.ApplyLabelTransformer) {
			return cell{}, status.Errorf(
				codes.InvalidArgument,
				`apply_label_transformer must match RE2([a-z0-9\-]+), but found %v`,
				filter.ApplyLabelTransformer,
			)
		}
		return cell{ts: c.ts, value: c.value, labels: []string{filter.ApplyLabelTransformer}}, nil
	default:
		return c, nil
	}
}

func includeCell(f *btpb.RowFilter, fam, col string, cell cell) (bool, error) {
	if f == nil {
		return true, nil
	}
	// TODO(dsymonds): Implement many more filters.
	switch f := f.Filter.(type) {
	case *btpb.RowFilter_CellsPerColumnLimitFilter:
		// Don't log, row-level filter
		return true, nil
	case *btpb.RowFilter_RowKeyRegexFilter:
		// Don't log, row-level filter
		return true, nil
	case *btpb.RowFilter_StripValueTransformer:
		// Don't log, cell-modifying filter
		return true, nil
	case *btpb.RowFilter_ApplyLabelTransformer:
		// Don't log, cell-modifying filter
		return true, nil
	default:
		log.Printf("WARNING: don't know how to handle filter of type %T (ignoring it)", f)
		return true, nil
	case *btpb.RowFilter_FamilyNameRegexFilter:
		rx, err := newRegexp([]byte(f.FamilyNameRegexFilter))
		if err != nil {
			return false, status.Errorf(codes.InvalidArgument, "Error in field 'family_name_regex_filter' : %v", err)
		}
		return rx.MatchString(fam), nil
	case *btpb.RowFilter_ColumnQualifierRegexFilter:
		rx, err := newRegexp(f.ColumnQualifierRegexFilter)
		if err != nil {
			return false, status.Errorf(codes.InvalidArgument, "Error in field 'column_qualifier_regex_filter' : %v", err)
		}
		return rx.MatchString(toUTF8([]byte(col))), nil
	case *btpb.RowFilter_ValueRegexFilter:
		rx, err := newRegexp(f.ValueRegexFilter)
		if err != nil {
			return false, status.Errorf(codes.InvalidArgument, "Error in field 'value_regex_filter' : %v", err)
		}
		return rx.Match(cell.value), nil
	case *btpb.RowFilter_ColumnRangeFilter:
		if fam != f.ColumnRangeFilter.FamilyName {
			return false, nil
		}
		// Start qualifier defaults to empty string closed
		inRangeStart := func() bool { return col >= "" }
		switch sq := f.ColumnRangeFilter.StartQualifier.(type) {
		case *btpb.ColumnRange_StartQualifierOpen:
			inRangeStart = func() bool { return col > string(sq.StartQualifierOpen) }
		case *btpb.ColumnRange_StartQualifierClosed:
			inRangeStart = func() bool { return col >= string(sq.StartQualifierClosed) }
		}
		// End qualifier defaults to no upper boundary
		inRangeEnd := func() bool { return true }
		switch eq := f.ColumnRangeFilter.EndQualifier.(type) {
		case *btpb.ColumnRange_EndQualifierClosed:
			inRangeEnd = func() bool { return col <= string(eq.EndQualifierClosed) }
		case *btpb.ColumnRange_EndQualifierOpen:
			inRangeEnd = func() bool { return col < string(eq.EndQualifierOpen) }
		}
		return inRangeStart() && inRangeEnd(), nil
	case *btpb.RowFilter_TimestampRangeFilter:
		// Lower bound is inclusive and defaults to 0, upper bound is exclusive and defaults to infinity.
		return cell.ts >= f.TimestampRangeFilter.StartTimestampMicros &&
			(f.TimestampRangeFilter.EndTimestampMicros == 0 || cell.ts < f.TimestampRangeFilter.EndTimestampMicros), nil
	case *btpb.RowFilter_ValueRangeFilter:
		v := cell.value
		// Start value defaults to empty string closed
		inRangeStart := func() bool { return bytes.Compare(v, []byte{}) >= 0 }
		switch sv := f.ValueRangeFilter.StartValue.(type) {
		case *btpb.ValueRange_StartValueOpen:
			inRangeStart = func() bool { return bytes.Compare(v, sv.StartValueOpen) > 0 }
		case *btpb.ValueRange_StartValueClosed:
			inRangeStart = func() bool { return bytes.Compare(v, sv.StartValueClosed) >= 0 }
		}
		// End value defaults to no upper boundary
		inRangeEnd := func() bool { return true }
		switch ev := f.ValueRangeFilter.EndValue.(type) {
		case *btpb.ValueRange_EndValueClosed:
			inRangeEnd = func() bool { return bytes.Compare(v, ev.EndValueClosed) <= 0 }
		case *btpb.ValueRange_EndValueOpen:
			inRangeEnd = func() bool { return bytes.Compare(v, ev.EndValueOpen) < 0 }
		}
		return inRangeStart() && inRangeEnd(), nil
	}
}

func toUTF8(bs []byte) string {
	var rs []rune
	for _, b := range bs {
		rs = append(rs, rune(b))
	}
	return string(rs)
}

func newRegexp(patBytes []byte) (*regexp.Regexp, error) {
	pat := toUTF8(patBytes)
	re, err := regexp.Compile("^" + pat + "$") // match entire target
	if err != nil {
		log.Printf("Bad pattern %q: %v", pat, err)
	}
	return re, err
}

func (s *server) MutateRow(ctx context.Context, req *btpb.MutateRowRequest) (*btpb.MutateRowResponse, error) {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.TableName)
	}
	fs := tbl.columnFamilies()
	r := tbl.mutableRow(string(req.RowKey))
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := applyMutations(tbl, r, req.Mutations, fs); err != nil {
		return nil, err
	}
	return &btpb.MutateRowResponse{}, nil
}

func (s *server) MutateRows(req *btpb.MutateRowsRequest, stream btpb.Bigtable_MutateRowsServer) error {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return status.Errorf(codes.NotFound, "table %q not found", req.TableName)
	}
	res := &btpb.MutateRowsResponse{Entries: make([]*btpb.MutateRowsResponse_Entry, len(req.Entries))}

	fs := tbl.columnFamilies()

	for i, entry := range req.Entries {
		r := tbl.mutableRow(string(entry.RowKey))
		r.mu.Lock()
		code, msg := int32(codes.OK), ""
		if err := applyMutations(tbl, r, entry.Mutations, fs); err != nil {
			code = int32(codes.Internal)
			msg = err.Error()
		}
		res.Entries[i] = &btpb.MutateRowsResponse_Entry{
			Index:  int64(i),
			Status: &statpb.Status{Code: code, Message: msg},
		}
		r.mu.Unlock()
	}
	return stream.Send(res)
}

func (s *server) CheckAndMutateRow(ctx context.Context, req *btpb.CheckAndMutateRowRequest) (*btpb.CheckAndMutateRowResponse, error) {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.TableName)
	}
	res := &btpb.CheckAndMutateRowResponse{}

	fs := tbl.columnFamilies()

	r := tbl.mutableRow(string(req.RowKey))
	r.mu.Lock()
	defer r.mu.Unlock()

	// Figure out which mutation to apply.
	whichMut := false
	if req.PredicateFilter == nil {
		// Use true_mutations iff row contains any cells.
		whichMut = !r.isEmpty()
	} else {
		// Use true_mutations iff any cells in the row match the filter.
		// TODO(dsymonds): This could be cheaper.
		nr := r.copy()

		// TODO(dsymonds, odeke-em): examine if filterRow here should be checked:
		// with: match, err := filterRow(req.PredicateFilter, nr)
		filterRow(req.PredicateFilter, nr)
		whichMut = !nr.isEmpty()
	}
	res.PredicateMatched = whichMut
	muts := req.FalseMutations
	if whichMut {
		muts = req.TrueMutations
	}

	if err := applyMutations(tbl, r, muts, fs); err != nil {
		return nil, err
	}
	return res, nil
}

// applyMutations applies a sequence of mutations to a row.
// fam should be a snapshot of the keys of tbl.families.
// It assumes r.mu is locked.
func applyMutations(tbl *table, r *row, muts []*btpb.Mutation, fs map[string]*columnFamily) error {
	for _, mut := range muts {
		switch mut := mut.Mutation.(type) {
		default:
			return fmt.Errorf("can't handle mutation type %T", mut)
		case *btpb.Mutation_SetCell_:
			set := mut.SetCell
			if _, ok := fs[set.FamilyName]; !ok {
				return fmt.Errorf("unknown family %q", set.FamilyName)
			}
			ts := set.TimestampMicros
			if ts == -1 { // bigtable.ServerTime
				ts = newTimestamp()
			}
			if !tbl.validTimestamp(ts) {
				return fmt.Errorf("invalid timestamp %d", ts)
			}
			fam := set.FamilyName
			col := string(set.ColumnQualifier)

			newCell := cell{ts: ts, value: set.Value}
			f := r.getOrCreateFamily(fam, fs[fam].order)
			f.cells[col] = appendOrReplaceCell(f.cellsByColumn(col), newCell)
		case *btpb.Mutation_DeleteFromColumn_:
			del := mut.DeleteFromColumn
			if _, ok := fs[del.FamilyName]; !ok {
				return fmt.Errorf("unknown family %q", del.FamilyName)
			}
			fam := del.FamilyName
			col := string(del.ColumnQualifier)
			if _, ok := r.families[fam]; ok {
				cs := r.families[fam].cells[col]
				if del.TimeRange != nil {
					tsr := del.TimeRange
					if !tbl.validTimestamp(tsr.StartTimestampMicros) {
						return fmt.Errorf("invalid timestamp %d", tsr.StartTimestampMicros)
					}
					if !tbl.validTimestamp(tsr.EndTimestampMicros) && tsr.EndTimestampMicros != 0 {
						return fmt.Errorf("invalid timestamp %d", tsr.EndTimestampMicros)
					}
					if tsr.StartTimestampMicros >= tsr.EndTimestampMicros && tsr.EndTimestampMicros != 0 {
						return fmt.Errorf("inverted or invalid timestamp range [%d, %d]", tsr.StartTimestampMicros, tsr.EndTimestampMicros)
					}

					// Find half-open interval to remove.
					// Cells are in descending timestamp order,
					// so the predicates to sort.Search are inverted.
					si, ei := 0, len(cs)
					if tsr.StartTimestampMicros > 0 {
						ei = sort.Search(len(cs), func(i int) bool { return cs[i].ts < tsr.StartTimestampMicros })
					}
					if tsr.EndTimestampMicros > 0 {
						si = sort.Search(len(cs), func(i int) bool { return cs[i].ts < tsr.EndTimestampMicros })
					}
					if si < ei {
						copy(cs[si:], cs[ei:])
						cs = cs[:len(cs)-(ei-si)]
					}
				} else {
					cs = nil
				}
				if len(cs) == 0 {
					delete(r.families[fam].cells, col)
					colNames := r.families[fam].colNames
					i := sort.Search(len(colNames), func(i int) bool { return colNames[i] >= col })
					if i < len(colNames) && colNames[i] == col {
						r.families[fam].colNames = append(colNames[:i], colNames[i+1:]...)
					}
					if len(r.families[fam].cells) == 0 {
						delete(r.families, fam)
					}
				} else {
					r.families[fam].cells[col] = cs
				}
			}
		case *btpb.Mutation_DeleteFromRow_:
			r.families = make(map[string]*family)
		case *btpb.Mutation_DeleteFromFamily_:
			fampre := mut.DeleteFromFamily.FamilyName
			delete(r.families, fampre)
		}
	}
	return nil
}

func maxTimestamp(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

func newTimestamp() int64 {
	ts := time.Now().UnixNano() / 1e3
	ts -= ts % 1000 // round to millisecond granularity
	return ts
}

func appendOrReplaceCell(cs []cell, newCell cell) []cell {
	replaced := false
	for i, cell := range cs {
		if cell.ts == newCell.ts {
			cs[i] = newCell
			replaced = true
			break
		}
	}
	if !replaced {
		cs = append(cs, newCell)
	}
	sort.Sort(byDescTS(cs))
	return cs
}

func (s *server) ReadModifyWriteRow(ctx context.Context, req *btpb.ReadModifyWriteRowRequest) (*btpb.ReadModifyWriteRowResponse, error) {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "table %q not found", req.TableName)
	}

	fs := tbl.columnFamilies()

	rowKey := string(req.RowKey)
	r := tbl.mutableRow(rowKey)
	resultRow := newRow(rowKey) // copy of updated cells

	// This must be done before the row lock, acquired below, is released.
	r.mu.Lock()
	defer r.mu.Unlock()
	// Assume all mutations apply to the most recent version of the cell.
	// TODO(dsymonds): Verify this assumption and document it in the proto.
	for _, rule := range req.Rules {
		if _, ok := fs[rule.FamilyName]; !ok {
			return nil, fmt.Errorf("unknown family %q", rule.FamilyName)
		}

		fam := rule.FamilyName
		col := string(rule.ColumnQualifier)
		isEmpty := false
		f := r.getOrCreateFamily(fam, fs[fam].order)
		cs := f.cells[col]
		isEmpty = len(cs) == 0

		ts := newTimestamp()
		var newCell, prevCell cell
		if !isEmpty {
			cells := r.families[fam].cells[col]
			prevCell = cells[0]

			// ts is the max of now or the prev cell's timestamp in case the
			// prev cell is in the future
			ts = maxTimestamp(ts, prevCell.ts)
		}

		switch rule := rule.Rule.(type) {
		default:
			return nil, fmt.Errorf("unknown RMW rule oneof %T", rule)
		case *btpb.ReadModifyWriteRule_AppendValue:
			newCell = cell{ts: ts, value: append(prevCell.value, rule.AppendValue...)}
		case *btpb.ReadModifyWriteRule_IncrementAmount:
			var v int64
			if !isEmpty {
				prevVal := prevCell.value
				if len(prevVal) != 8 {
					return nil, fmt.Errorf("increment on non-64-bit value")
				}
				v = int64(binary.BigEndian.Uint64(prevVal))
			}
			v += rule.IncrementAmount
			var val [8]byte
			binary.BigEndian.PutUint64(val[:], uint64(v))
			newCell = cell{ts: ts, value: val[:]}
		}

		// Store the new cell
		f.cells[col] = appendOrReplaceCell(f.cellsByColumn(col), newCell)

		// Store a copy for the result row
		resultFamily := resultRow.getOrCreateFamily(fam, fs[fam].order)
		resultFamily.cellsByColumn(col)           // create the column
		resultFamily.cells[col] = []cell{newCell} // overwrite the cells
	}

	// Build the response using the result row
	res := &btpb.Row{
		Key:      req.RowKey,
		Families: make([]*btpb.Family, len(resultRow.families)),
	}

	for i, family := range resultRow.sortedFamilies() {
		res.Families[i] = &btpb.Family{
			Name:    family.name,
			Columns: make([]*btpb.Column, len(family.colNames)),
		}

		for j, colName := range family.colNames {
			res.Families[i].Columns[j] = &btpb.Column{
				Qualifier: []byte(colName),
				Cells: []*btpb.Cell{{
					TimestampMicros: family.cells[colName][0].ts,
					Value:           family.cells[colName][0].value,
				}},
			}
		}
	}
	return &btpb.ReadModifyWriteRowResponse{Row: res}, nil
}

func (s *server) SampleRowKeys(req *btpb.SampleRowKeysRequest, stream btpb.Bigtable_SampleRowKeysServer) error {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return status.Errorf(codes.NotFound, "table %q not found", req.TableName)
	}

	tbl.mu.RLock()
	defer tbl.mu.RUnlock()

	// The return value of SampleRowKeys is very loosely defined. Return at least the
	// final row key in the table and choose other row keys randomly.
	var offset int64
	var err error
	i := 0
	tbl.rows.Ascend(func(it btree.Item) bool {
		row := it.(*row)
		if i == tbl.rows.Len()-1 || rand.Int31n(100) == 0 {
			resp := &btpb.SampleRowKeysResponse{
				RowKey:      []byte(row.key),
				OffsetBytes: offset,
			}
			err = stream.Send(resp)
			if err != nil {
				return false
			}
		}
		offset += int64(row.size())
		i++
		return true
	})
	return err
}

// needGC is invoked whenever the server needs gcloop running.
func (s *server) needGC() {
	s.mu.Lock()
	if s.gcc == nil {
		s.gcc = make(chan int)
		go s.gcloop(s.gcc)
	}
	s.mu.Unlock()
}

func (s *server) gcloop(done <-chan int) {
	const (
		minWait = 500  // ms
		maxWait = 1500 // ms
	)

	for {
		// Wait for a random time interval.
		d := time.Duration(minWait+rand.Intn(maxWait-minWait)) * time.Millisecond
		select {
		case <-time.After(d):
		case <-done:
			return // server has been closed
		}

		// Do a GC pass over all tables.
		var tables []*table
		s.mu.Lock()
		for _, tbl := range s.tables {
			tables = append(tables, tbl)
		}
		s.mu.Unlock()
		for _, tbl := range tables {
			tbl.gc()
		}
	}
}

type table struct {
	mu       sync.RWMutex
	counter  uint64                   // increment by 1 when a new family is created
	families map[string]*columnFamily // keyed by plain family name
	rows     *btree.BTree             // indexed by row key
}

const btreeDegree = 16

func newTable(ctr *btapb.CreateTableRequest) *table {
	fams := make(map[string]*columnFamily)
	c := uint64(0)
	if ctr.Table != nil {
		for id, cf := range ctr.Table.ColumnFamilies {
			fams[id] = &columnFamily{
				name:   ctr.Parent + "/columnFamilies/" + id,
				order:  c,
				gcRule: cf.GcRule,
			}
			c++
		}
	}
	return &table{
		families: fams,
		counter:  c,
		rows:     btree.New(btreeDegree),
	}
}

func (t *table) validTimestamp(ts int64) bool {
	if ts < minValidMilliSeconds || ts > maxValidMilliSeconds {
		return false
	}

	// Assume millisecond granularity is required.
	return ts%1000 == 0
}

func (t *table) columnFamilies() map[string]*columnFamily {
	cp := make(map[string]*columnFamily)
	t.mu.RLock()
	for fam, cf := range t.families {
		cp[fam] = cf
	}
	t.mu.RUnlock()
	return cp
}

func (t *table) mutableRow(key string) *row {
	bkey := btreeKey(key)
	// Try fast path first.
	t.mu.RLock()
	i := t.rows.Get(bkey)
	t.mu.RUnlock()
	if i != nil {
		return i.(*row)
	}

	// We probably need to create the row.
	t.mu.Lock()
	defer t.mu.Unlock()
	i = t.rows.Get(bkey)
	if i != nil {
		return i.(*row)
	}
	r := newRow(key)
	t.rows.ReplaceOrInsert(r)
	return r
}

func (t *table) gc() {
	// This method doesn't add or remove rows, so we only need a read lock for the table.
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Gather GC rules we'll apply.
	rules := make(map[string]*btapb.GcRule) // keyed by "fam"
	for fam, cf := range t.families {
		if cf.gcRule != nil {
			rules[fam] = cf.gcRule
		}
	}
	if len(rules) == 0 {
		return
	}

	t.rows.Ascend(func(i btree.Item) bool {
		r := i.(*row)
		r.mu.Lock()
		r.gc(rules)
		r.mu.Unlock()
		return true
	})
}

type byRowKey []*row

func (b byRowKey) Len() int           { return len(b) }
func (b byRowKey) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byRowKey) Less(i, j int) bool { return b[i].key < b[j].key }

type row struct {
	key string

	mu       sync.Mutex
	families map[string]*family // keyed by family name
}

func newRow(key string) *row {
	return &row{
		key:      key,
		families: make(map[string]*family),
	}
}

// copy returns a copy of the row.
// Cell values are aliased.
// r.mu should be held.
func (r *row) copy() *row {
	nr := newRow(r.key)
	for _, fam := range r.families {
		nr.families[fam.name] = &family{
			name:     fam.name,
			order:    fam.order,
			colNames: fam.colNames,
			cells:    make(map[string][]cell),
		}
		for col, cs := range fam.cells {
			// Copy the []cell slice, but not the []byte inside each cell.
			nr.families[fam.name].cells[col] = append([]cell(nil), cs...)
		}
	}
	return nr
}

// isEmpty returns true if a row doesn't contain any cell
func (r *row) isEmpty() bool {
	for _, fam := range r.families {
		for _, cs := range fam.cells {
			if len(cs) > 0 {
				return false
			}
		}
	}
	return true
}

// sortedFamilies returns a column family set
// sorted in ascending creation order in a row.
func (r *row) sortedFamilies() []*family {
	var families []*family
	for _, fam := range r.families {
		families = append(families, fam)
	}
	sort.Sort(byCreationOrder(families))
	return families
}

func (r *row) getOrCreateFamily(name string, order uint64) *family {
	if _, ok := r.families[name]; !ok {
		r.families[name] = &family{
			name:  name,
			order: order,
			cells: make(map[string][]cell),
		}
	}
	return r.families[name]
}

// gc applies the given GC rules to the row.
// r.mu should be held.
func (r *row) gc(rules map[string]*btapb.GcRule) {
	for _, fam := range r.families {
		rule, ok := rules[fam.name]
		if !ok {
			continue
		}
		for col, cs := range fam.cells {
			r.families[fam.name].cells[col] = applyGC(cs, rule)
		}
	}
}

// size returns the total size of all cell values in the row.
func (r *row) size() int {
	size := 0
	for _, fam := range r.families {
		for _, cells := range fam.cells {
			for _, cell := range cells {
				size += len(cell.value)
			}
		}
	}
	return size
}

// Less implements btree.Less.
func (r *row) Less(i btree.Item) bool {
	return r.key < i.(*row).key
}

// btreeKey returns a row for use as a key into the BTree.
func btreeKey(s string) *row { return &row{key: s} }

func (r *row) String() string {
	return r.key
}

var gcTypeWarn sync.Once

// applyGC applies the given GC rule to the cells.
func applyGC(cells []cell, rule *btapb.GcRule) []cell {
	switch rule := rule.Rule.(type) {
	default:
		// TODO(dsymonds): Support GcRule_Intersection_
		gcTypeWarn.Do(func() {
			log.Printf("Unsupported GC rule type %T", rule)
		})
	case *btapb.GcRule_Union_:
		for _, sub := range rule.Union.Rules {
			cells = applyGC(cells, sub)
		}
		return cells
	case *btapb.GcRule_MaxAge:
		// Timestamps are in microseconds.
		cutoff := time.Now().UnixNano() / 1e3
		cutoff -= rule.MaxAge.Seconds * 1e6
		cutoff -= int64(rule.MaxAge.Nanos) / 1e3
		// The slice of cells in in descending timestamp order.
		// This sort.Search will return the index of the first cell whose timestamp is chronologically before the cutoff.
		si := sort.Search(len(cells), func(i int) bool { return cells[i].ts < cutoff })
		if si < len(cells) {
			log.Printf("bttest: GC MaxAge(%v) deleted %d cells.", rule.MaxAge, len(cells)-si)
		}
		return cells[:si]
	case *btapb.GcRule_MaxNumVersions:
		n := int(rule.MaxNumVersions)
		if len(cells) > n {
			cells = cells[:n]
		}
		return cells
	}
	return cells
}

type family struct {
	name     string            // Column family name
	order    uint64            // Creation order of column family
	colNames []string          // Column names are sorted in lexicographical ascending order
	cells    map[string][]cell // Keyed by column name; cells are in descending timestamp order
}

type byCreationOrder []*family

func (b byCreationOrder) Len() int           { return len(b) }
func (b byCreationOrder) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byCreationOrder) Less(i, j int) bool { return b[i].order < b[j].order }

// cellsByColumn adds the column name to colNames set if it does not exist
// and returns all cells within a column
func (f *family) cellsByColumn(name string) []cell {
	if _, ok := f.cells[name]; !ok {
		f.colNames = append(f.colNames, name)
		sort.Strings(f.colNames)
	}
	return f.cells[name]
}

type cell struct {
	ts     int64
	value  []byte
	labels []string
}

type byDescTS []cell

func (b byDescTS) Len() int           { return len(b) }
func (b byDescTS) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byDescTS) Less(i, j int) bool { return b[i].ts > b[j].ts }

type columnFamily struct {
	name   string
	order  uint64 // Creation order of column family
	gcRule *btapb.GcRule
}

func (c *columnFamily) proto() *btapb.ColumnFamily {
	return &btapb.ColumnFamily{
		GcRule: c.gcRule,
	}
}

func toColumnFamilies(families map[string]*columnFamily) map[string]*btapb.ColumnFamily {
	fs := make(map[string]*btapb.ColumnFamily)
	for k, v := range families {
		fs[k] = v.proto()
	}
	return fs
}
