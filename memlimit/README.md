memlimit provides a basic Linux docker container that allows running lantern
(or anything else) with a limited amount of memory.

1. Install [Docker](https://www.docker.com/)
2. Inside of the memlimit folder, run `docker build -t memlimit .` to create the memlimit image
3. Inside of the flashlight folder, run `make linux`

Now you can run the Linux version of Lantern with memory limiting enabled. For
example to limit it to 10 MB, run it like this:

```
docker run -m 10m -v <path_to_flashlight>/lantern-linux:/lantern memlimit /lantern -headless
```

This command does the following:

- `docker run` runs a container
- `-m 10m` limits memory for the docker container to 10 MB
- `-v <path_to_flashlight>/lantern-linux:/lantern` mounts the `lantern-linux` binary from the host filesystem to `/lantern` within the container
- `memlimit` is the name of the container
- `/lantern -headless` runs this command in the container
