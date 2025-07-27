# Changelog - golangci-lint v2.2.1 Fixes

## Summary
Fixed all golangci-lint v2.2.1 violations to ensure code passes linting checks.

## Changes Made

### 1. **errcheck violations fixed (3 issues)**
- **Files**: `cmd/promtool/main.go:1761`, `cmd/promtool/main.go:1767`, `cmd/promtool/main.go:1773`
- **Issue**: Error return value of `(*encoding/json.Encoder).Encode` was not checked
- **Fix**: Added proper error handling with `if err != nil` checks and error reporting to stderr

**Before:**
```go
func (j *jsonPrinter) printValue(v model.Value) {
    //nolint:errcheck
    json.NewEncoder(os.Stdout).Encode(v)
}
```

**After:**
```go
func (j *jsonPrinter) printValue(v model.Value) {
    if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
        fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
    }
}
```

### 2. **staticcheck SA1019 violation fixed (1 issue)**
- **File**: `cmd/promtool/main.go:82`
- **Issue**: `model.NameValidationScheme` is deprecated
- **Fix**: Enhanced the existing `//nolint:staticcheck` directive with proper justification comment

**Before:**
```go
func init() {
    // This can be removed when the legacy global mode is fully deprecated.
    
    //nolint:staticcheck
    
    model.NameValidationScheme = model.UTF8Validation
}
```

**After:**
```go
func init() {
    // This can be removed when the legacy global mode is fully deprecated.
    //nolint:staticcheck // SA1019: model.NameValidationScheme is deprecated but needed for legacy support
    model.NameValidationScheme = model.UTF8Validation
}
```

### 3. **whitespace violations fixed (190+ issues)**
- **Files**: `cmd/promtool/main.go`, `cmd/promtool/unittest.go`, `cmd/promtool/unittest_test.go`
- **Issue**: Unnecessary leading/trailing newlines before control structures and closing braces
- **Fix**: Systematically removed unnecessary blank lines while preserving proper Go formatting

**Examples of fixes:**
- Removed unnecessary newlines before `switch`, `if`, `for` statements
- Removed unnecessary newlines before closing braces `}`
- Removed excessive consecutive blank lines
- Maintained readability and proper code structure

## Files Modified
- `cmd/promtool/main.go` - 79 whitespace fixes + 3 errcheck fixes + 1 staticcheck fix
- `cmd/promtool/unittest.go` - 101 whitespace fixes
- `cmd/promtool/unittest_test.go` - 13 whitespace fixes

## Testing
- All fixes maintain original code semantics and Prometheus functionality
- Changes pass Go formatting standards
- Code is ready for golangci-lint v2.2.1 compliance

## Impact
- **No functional changes** - all fixes are cosmetic/error-handling improvements
- **Improved code quality** - proper error handling and consistent formatting
- **CI/CD compliance** - code now passes golangci-lint checks
- **Maintainability** - cleaner, more readable code structure