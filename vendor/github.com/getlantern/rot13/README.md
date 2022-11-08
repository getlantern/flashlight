Rot13 encoder/decoder

See sample usage [here](https://github.com/getlantern/flashlight/blob/8ddc8a9e7571bb702bd6006843a64560cec9be8b/config/config.go#L221)

For a standalone decode/encode, supply the input/output file path as env variables and run:

    INFILE=~/.lantern/global.yaml \
      OUTFILE=~/.lantern/expanded_global.yaml \
      go test -v -run TestFunctional
