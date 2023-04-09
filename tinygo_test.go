package board_test

import (
	"bytes"
	"flag"
	"os/exec"
	"testing"
)

var boards = []string{
	// Please keep this list sorted!
	"gameboy-advance",
	"gopher-badge",
	"mch2022",
	"pybadge",
	"simulator",
}

func isXtensa(board string) bool {
	return board == "mch2022"
}

var flagXtensa = flag.Bool("xtensa", false, "test Xtensa based boards")

func TestBoards(t *testing.T) {
	for _, board := range boards {
		board := board
		t.Run(board, func(t *testing.T) {
			if isXtensa(board) && !*flagXtensa {
				t.Skip("skipping Xtensa board:", board)
			}
			t.Parallel()
			outbuf := &bytes.Buffer{}
			var cmd *exec.Cmd
			if board == "simulator" {
				cmd = exec.Command("go", "build", "-o="+t.TempDir()+"/output", "./testdata/smoketest.go")
			} else {
				cmd = exec.Command("tinygo", "build", "-o="+t.TempDir()+"/output", "-target="+board, "./testdata/smoketest.go")
			}
			cmd.Stderr = outbuf
			cmd.Stdout = outbuf
			err := cmd.Run()
			if err != nil {
				t.Errorf("failed to compile smoke test: %s\n%s", err, outbuf.String())
			}
		})
	}
}
