package mcpserver

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"
)

func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var mu sync.Mutex
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, ok := s.handleMessage(ctx, scanner.Bytes())
		if !ok {
			continue
		}
		mu.Lock()
		_, werr := fmt.Fprintf(out, "%s\n", resp)
		mu.Unlock()
		if werr != nil {
			return werr
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}
