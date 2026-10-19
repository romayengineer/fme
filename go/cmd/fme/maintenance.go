package main

import (
	"fmt"

	"github.com/romayengineer/fme/go/app"
)

func printDescriptionStats(stats app.DescriptionStats) {
	fmt.Println("data/__Details description report")
	fmt.Printf("  total listings:    %d\n", stats.Total)
	fmt.Printf("  with description:  %d (%.2f%%)\n", stats.WithDescription, stats.WithDescriptionPercent())
	fmt.Printf("  unavailable:       %d (%.2f%%)  deleted listings (skipped by pull-description)\n", stats.Unavailable, stats.UnavailablePercent())
	fmt.Printf("  empty (null):      %d (%.2f%%)\n", stats.Empty, stats.EmptyPercent())
}
