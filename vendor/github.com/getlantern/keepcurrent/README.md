# keepcurrent

A library to periodically check for changes from the source and update a set of sinks. Optionally the sinks can be initialized from a different source for once.

One typical user case is to keep both the in memory configuration and local config file in sync with the remote one, and load the local config file when the program starts up to speed up a bit.

