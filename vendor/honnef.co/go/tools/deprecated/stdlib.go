package deprecated

type Deprecation struct {
	DeprecatedSince           int
	AlternativeAvailableSince int
}

var Stdlib = map[string]Deprecation{
	"image/jpeg.Reader": {4, 0},
	// FIXME(dh): AllowBinary isn't being detected as deprecated
	// because the comment has a newline right after "Deprecated:"
	"go/build.AllowBinary":                      {7, 7},
	"(archive/zip.FileHeader).CompressedSize":   {1, 1},
	"(archive/zip.FileHeader).UncompressedSize": {1, 1},
	"(go/doc.Package).Bugs":                     {1, 1},
	"os.SEEK_SET":                               {7, 7},
	"os.SEEK_CUR":                               {7, 7},
	"os.SEEK_END":                               {7, 7},
	"(net.Dialer).Cancel":                       {7, 7},
	"runtime.CPUProfile":                        {9, 0},
	"compress/flate.ReadError":                  {6, 6},
	"compress/flate.WriteError":                 {6, 6},
	"path/filepath.HasPrefix":                   {0, 0},
	"(net/http.Transport).Dial":                 {7, 7},
	"(*net/http.Transport).CancelRequest":       {6, 5},
	"net/http.ErrWriteAfterFlush":               {7, 0},
	"net/http.ErrHeaderTooLong":                 {8, 0},
	"net/http.ErrShortBody":                     {8, 0},
	"net/http.ErrMissingContentLength":          {8, 0},
	"net/http/httputil.ErrPersistEOF":           {0, 0},
	"net/http/httputil.ErrClosed":               {0, 0},
	"net/http/httputil.ErrPipeline":             {0, 0},
	"net/http/httputil.ServerConn":              {0, 0},
	"net/http/httputil.NewServerConn":           {0, 0},
	"net/http/httputil.ClientConn":              {0, 0},
	"net/http/httputil.NewClientConn":           {0, 0},
	"net/http/httputil.NewProxyClientConn":      {0, 0},
	"(net/http.Request).Cancel":                 {7, 7},
	"(text/template/parse.PipeNode).Line":       {1, 1},
	"(text/template/parse.ActionNode).Line":     {1, 1},
	"(text/template/parse.BranchNode).Line":     {1, 1},
	"(text/template/parse.TemplateNode).Line":   {1, 1},
	"database/sql/driver.ColumnConverter":       {9, 9},
	"database/sql/driver.Execer":                {8, 8},
	"database/sql/driver.Queryer":               {8, 8},
	"(database/sql/driver.Conn).Begin":          {8, 8},
	"(database/sql/driver.Stmt).Exec":           {8, 8},
	"(database/sql/driver.Stmt).Query":          {8, 8},
	"syscall.StringByteSlice":                   {1, 1},
	"syscall.StringBytePtr":                     {1, 1},
	"syscall.StringSlicePtr":                    {1, 1},
	"syscall.StringToUTF16":                     {1, 1},
	"syscall.StringToUTF16Ptr":                  {1, 1},
}
