# testreport

generate report by `go test -json ./... -covermode=atomic -coverprofile /tmp/coverage.out`

## install

```shell
go install github.com/huskar-t/testreport@latest
```

## usage

```shell
Usage of testreport:
  -o string
        output file default stdout
  -t string
        a(all) c(coverage) t (test) v(vet) (default "a")
```