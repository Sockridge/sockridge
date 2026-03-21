package main

import (
	"context"
	"fmt"
	"log"

	sockridge "github.com/Sockridge/sockridge/sdk/go/sockridge"
)

func main() {
    r := sockridge.New("http://sockridge:9000")
    if err := r.Login("", ""); err != nil {
        log.Fatal(err)
    }
    fmt.Println("logged in")

    agents, err := r.Search(context.Background(), []string{"nlp"}, 10)
    if err != nil {
        log.Fatal(err)
    }
    for _, a := range agents {
        fmt.Printf("%s — %s\n", a.ID, a.Name)
    }
}
