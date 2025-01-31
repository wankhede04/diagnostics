package erigon_node

import (
	"context"
	"encoding/binary"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"

	"github.com/ledgerwatch/diagnostics/internal"
)

// Demonstration of the working with the Erigon database remotely on the example of getting information
// about past reorganisation of the chain
const (
	headersDb    = "chaindata"
	headersTable = "Header"
	maxCount     = 1000
)

// FindReorgs - Go through "Header" table and look for entries with the same block number but different hashes
func (c *NodeClient) FindReorgs(ctx context.Context,
	writer http.ResponseWriter,
	template *template.Template,
	requestChannel chan *internal.NodeRequest) {
	start := time.Now()
	var err error

	rc := NewRemoteCursor(c, requestChannel)
	if err = rc.Init(headersDb, headersTable, nil); err != nil {
		fmt.Fprintf(writer, "Create remote cursor: %v", err)
		return
	}

	total, wrongBlocks, errors := c.findReorgsInternally(ctx, template, rc)
	for _, err := range errors {
		if err != nil {
			fmt.Fprintf(writer, "%v\n", err)
		}
	}

	fmt.Fprintf(writer, "Reorg iterator: %d, total scanned %s\n", len(total), time.Since(start))
	fmt.Fprintf(writer, "Reorg iterator: %d, wrong blocks\n", len(wrongBlocks))
}

func (c *NodeClient) executeFlush(writer io.Writer,
	template *template.Template,
	name string, data any) error {
	if err := template.ExecuteTemplate(writer, name, data); err != nil {
		return err
	}
	if f, ok := writer.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// findReorgsInternally - searching for reorgs,
// return back total blocks set and wrong blocks
// if there are errors in the middle of processing will return back
// slice of errors
func (c *NodeClient) findReorgsInternally(ctx context.Context,
	template *template.Template,
	rc *RemoteCursor,
) (map[uint64][]byte, []uint64, []error) {
	var errors []error
	set := make(map[uint64][]byte)
	var wrongBlocks []uint64

	var k []byte

	var iterator int
	var err error
	for k, _, err = rc.Next(); err == nil && k != nil; k, _, err = rc.Next() {
		select {
		case <-ctx.Done():
			return nil, nil, []error{fmt.Errorf("Interrupted\n")}
		default:
		}

		if len(k) == 0 {
			continue
		}

		bn := binary.BigEndian.Uint64(k[:8])
		_, found := set[bn]
		if found {
			if template != nil {
				if err := c.executeFlush(nil, template, "reorg_block.html", bn); err != nil {
					errors = append(errors, fmt.Errorf("Executing reorg_block template: %v\n", err))
				}
			}
			wrongBlocks = append(wrongBlocks, bn)
		}
		set[bn] = k

		iterator++
		if iterator%maxCount == 0 {
			if template != nil {
				if err := c.executeFlush(nil, template, "reorg_block.html", bn); err != nil {
					errors = append(errors, fmt.Errorf("Executing reorg_spacer template: %v\n", err))
				}
			}
		}
	}
	if err != nil {
		errors = append(errors, err)
	}

	return set, wrongBlocks, errors
}
