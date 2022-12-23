package prefixgen

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuiltins tests parsing and execution of built-in functions. We test built-ins through the
// parser to ensure end-to-end testing of argument parsing. More thorough testing of the parser is
// left to TestParser.
func TestBuiltins(t *testing.T) {
	generate := func(t *testing.T, generator string) []byte {
		t.Helper()
		eval, err := parse(generator, Builtins)
		require.NoError(t, err)
		return eval()
	}

	t.Run("hex", func(t *testing.T) {
		out := generate(t, `v1.0 $hex(1a2b3cBEEF)`)
		require.Equal(t, []byte{0x1a, 0x2b, 0x3c, 0xbe, 0xef}, out)
	})

	t.Run("random_string", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			out := string(generate(t, `v1.0 $random_string(1, 10)`))
			require.GreaterOrEqual(t, len(out), 1)
			require.Less(t, len(out), 10)
		}
	})

	t.Run("random_string/custom_set", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			out := string(generate(t, `v1.0 $random_string(1, 10, aeiou)`))
			for _, r := range out {
				require.Contains(t, []rune("aeiou"), r)
			}
		}
	})

	t.Run("random_int", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			out := generate(t, `v1.0 $random_int(-10, 10)`)

			n, err := strconv.Atoi(string(out))
			require.NoError(t, err)
			require.GreaterOrEqual(t, n, -10)
			require.Less(t, n, 10)
		}
	})

	t.Run("random_bytes", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			out := generate(t, `v1.0 $random_bytes(1, 10)`)
			require.GreaterOrEqual(t, len(out), 1)
			require.Less(t, len(out), 10)
		}
	})

	t.Run("random_bytes/custom_set", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			out := generate(t, `v1.0 $random_bytes(1, 10, 010203A1A2A3)`)
			for _, b := range out {
				require.Contains(t, []byte{0x01, 0x02, 0x03, 0xa1, 0xa2, 0xa3}, b)
			}
		}
	})
}
