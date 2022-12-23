// Package prefixgen defines a language for generating connection prefixes. The motivating use case
// was to create prefixes for use in Shadowsocks clients. However, the prefix generators offered by
// this package are just byte string generators.
//
// The entrypoint to this package is the New function.
package prefixgen

// PrefixGen is used to produces connection prefixes. This is a generator compiled to a Go function
// See the New function.
type PrefixGen func() []byte

// New creates a new PrefixGen based on a generator. The generator is a rudimentary program, with a
// grammar defined in EBNF as:
//
//	letter = "A" | "B" | "C" | "D" | "E" | "F" | "G"
//	  | "H" | "I" | "J" | "K" | "L" | "M" | "N"
//	  | "O" | "P" | "Q" | "R" | "S" | "T" | "U"
//	  | "V" | "W" | "X" | "Y" | "Z" | "a" | "b"
//	  | "c" | "d" | "e" | "f" | "g" | "h" | "i"
//	  | "j" | "k" | "l" | "m" | "n" | "o" | "p"
//	  | "q" | "r" | "s" | "t" | "u" | "v" | "w"
//	  | "x" | "y" | "z" ;
//	digit = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;
//	whitespace = " " | "\t" | "\n" | "\r" ;
//	char = letter | digit | whitespace
//
//	number = digit, { digit } ;
//	string = char, { char } ;
//	identifier = letter, { letter | "_" } ;
//
//	version = "v", number , ".", number ;
//	arg list = string, { ",", " ", string } ;
//	function call = "$", identifier, "(", [ arg list ], ")" ;
//
//	generator = version, " ", { string | whitespace | function call }
//
// where the function calls reference a limited set of built-in functions defined in Builtins.
// Nested function calls are not supported.
//
// Generators begin with a version specifier. This specifier is used to make parsing decisions, but
// is left out of the output prefix.
//
// Example generators:
//
//	httpGet = `v1.0 GET /$random_string(5, 10) HTTP/1.1`
//
//	dnsOverTCP = `v1.0 $hex(05DC)$random_bytes(2, 3)$hex(0120)`
func New(generator string) (PrefixGen, error) {
	return parse(generator, Builtins)
}
