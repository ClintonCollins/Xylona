package supervisor

import (
	"bytes"
	"io"
	"slices"
	"strings"
)

type redactedReader struct {
	reader  io.Reader
	secrets []string
	pending string
	ready   bytes.Buffer
	err     error
}

func (c *Command) redactedOutputReader(reader io.Reader) io.Reader {
	if c.ServiceID() != "valheim" {
		return reader
	}
	c.RLock()
	secrets := slices.Clone(c.redactValues)
	c.RUnlock()
	if len(secrets) == 0 {
		return reader
	}
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	return &redactedReader{reader: reader, secrets: secrets}
}

func (r *redactedReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	for r.ready.Len() == 0 && r.err == nil {
		input := make([]byte, consoleReadBufferBytes)
		count, errRead := r.reader.Read(input)
		r.pending += string(input[:count])
		r.flush(errRead != nil)
		r.err = errRead
	}
	if r.ready.Len() > 0 {
		count := copy(buffer, r.ready.Next(len(buffer)))
		return count, nil
	}
	return 0, r.err
}

// Buffer partial credentials so secrets split across reads are redacted.
func (r *redactedReader) flush(final bool) {
	redacted := make([]byte, 0, len(r.pending))
	consumed := 0
consume:
	for consumed < len(r.pending) {
		matched := false
		for _, secret := range r.secrets {
			if secret == "" {
				continue
			}
			remaining := r.pending[consumed:]
			if !final && len(remaining) < len(secret) && strings.HasPrefix(secret, remaining) {
				break consume
			}
			if strings.HasPrefix(remaining, secret) {
				redacted = append(redacted, "[redacted]"...)
				consumed += len(secret)
				matched = true
				break
			}
		}
		if !matched {
			redacted = append(redacted, r.pending[consumed])
			consumed++
		}
	}
	r.ready.Write(redacted)
	r.pending = r.pending[consumed:]
	if final {
		clear(r.secrets)
		r.secrets = nil
	}
}
