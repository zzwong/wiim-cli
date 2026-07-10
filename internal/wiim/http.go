package wiim

import (
	"fmt"
	"io"
	"math"
)

const (
	wiimAPIResponseLimit      int64 = 1 << 20
	spotifyAPIResponseLimit   int64 = 1 << 20
	spotifyTokenResponseLimit int64 = 64 << 10
)

func readLimitedResponse(reader io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("response limit must be non-negative")
	}
	if limit == math.MaxInt64 {
		return nil, fmt.Errorf("response limit must be less than math.MaxInt64")
	}

	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}
