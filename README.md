# testreport

generate report by `go test -json ./... -covermode=atomic -coverprofile /tmp/coverage.out`

## install

go install github.com/huskar-t/testreport

## usage

```bash
Usage of testreport:
  -o string
        output file default stdout
  -t string
        a(all) c(coverage) t (test) v(vet) (default "a")
```