# Simple PLY file reader

Golang simple PLY (binary_little_endian only) file reader.

## Example:

Read first point

```go
func main() {
    ply, err := plyfile.Open("./reconstruction_dense.ply")
    if err != nil {
        log.Fatal(err)
    }
    defer ply.Close()

    r, err := ply.GetElementReader("vertex")
    if err == nil {
        point := plyfile.Point{}
        err := r.ReadFirst(&point)
        if err == nil {
            fmt.Println(point.String())
        }
    }
}
```

Read all points

```go
func main() {
	ply, err := plyfile.Open("./reconstruction_dense.ply")
	if err != nil {
		log.Fatal(err)
	}
	defer ply.Close()

	r, err := ply.GetElementReader("vertex")
	if err == nil {
		point := plyfile.Point{}

        for {
            _, err := r.ReadNext(&point)
            if err == io.EOF {
                break
            }
            fmt.Println(point.String())
        }
    }
}
```

Reflect point structure

```go
type MyPoint struct {
    X float64 `ply:"x"`
    Y float64 `ply:"y"`
    Z float64 `ply:"z"`
}

func main() {
	ply, err := plyfile.Open("./reconstruction_dense.ply")
	if err != nil {
		log.Fatal(err)
	}
	defer ply.Close()

	r, err := ply.GetElementReader("vertex")
	if err == nil {
		point := MyPoint{}

        for {
            _, err := r.ReadNext(&point)
            if err == io.EOF {
                break
            }
            fmt.Println(point)
        }
    }
}
```

Enjoy it ;)