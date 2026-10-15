# Shortcuts for the checks CI runs. Every recipe is a single plain command so
# it works the same under cmd.exe (make's shell on Windows when sh is not on
# PATH), Git Bash and Linux.

.PHONY: test race lint run

# test runs every test.
test:
	go test ./...

# race runs every test under the race detector, which needs cgo. CGO_ENABLED
# is exported for this target only, because it defaults to 0 on Windows.
race: export CGO_ENABLED = 1
race:
	go test -race ./...

# unformatted lists the Go files gofmt would change. It is evaluated only when
# lint uses it.
unformatted = $(shell gofmt -l .)

# lint runs go vet and fails if any file needs gofmt. The gofmt check is a make
# function rather than a shell test so it needs no particular shell. make
# expands a recipe before running it, so the gofmt check runs before vet.
lint:
	$(if $(unformatted),$(error files need gofmt: $(unformatted)))
	go vet ./...

# run starts the server on :8080 with the sample test form. Ctrl+C stops it.
run:
	go run ./cmd/server -items ./testdata/items
