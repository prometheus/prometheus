go-asap: ASAP smoothing

https://github.com/stanford-futuredata/ASAP

## Usage

```go
package main

import "github.com/errx/go-asap"

func main() {
	data := []float64{12751, 8767, 7005, 5257, 4189, 3236, 2817, 2527, 2406, 1961}
	smoothed := asap.Smooth(data, 3)
}
```
