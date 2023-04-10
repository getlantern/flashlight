Flashlight integration tester

This tool runs a series of integration tests against a local Flashlight client and a local http-proxy-lantern instance.

## Usage

    go run . -test <testName>
    // Runs the specified test. If no test name is specified, runs all tests.

    List of supported tests:
    - http
    - https
    - shadowsocks-no-multiplex
