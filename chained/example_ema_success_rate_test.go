package chained

import (
	"fmt"
	"math"

	"github.com/getlantern/ema"
)

func ExampleEMASuccessRateLow() {
	rate := ema.New(1, 0.1)
	fmt.Println("Doesn't respond to hiccups quickly enough")
	for i := 0; i < 100; i++ {
		rate.Update(1)
	}
	fmt.Printf("%.3f\t", rate.Update(1))
	fmt.Printf("%.3f\t", rate.Update(0))
	fmt.Printf("%.3f\t", rate.Update(0))
	fmt.Printf("%.3f\t", rate.Update(0))
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(1))
	}

	fmt.Println(".") // workaround for https://github.com/golang/go/issues/26460
	fmt.Println("Especially if there's only one failure")
	fmt.Printf("%.3f\t", rate.Update(0))
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(1))
	}

	fmt.Println(".")
	fmt.Println("Unstable dialer does keep getting a low score")
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(math.Mod(float64(i), 2)))
	}
	// Output:
	// Doesn't respond to hiccups quickly enough
	// 1.000	0.900	0.810	0.729	0.756	0.780	0.802	0.822	0.839	0.854	0.869	0.881	0.893	0.903	.
	// Especially if there's only one failure
	// 0.812	0.830	0.846	0.861	0.875	0.887	0.897	0.907	0.916	0.924	0.932	.
	// Unstable dialer does keep getting a low score
	// 0.838	0.853	0.768	0.790	0.711	0.740	0.665	0.699	0.628	0.665
}

func ExampleEMASuccessRateHigh() {
	rate := ema.New(1, 0.7)
	fmt.Println("Should recover from hiccups quickly")
	for i := 0; i < 100; i++ {
		rate.Update(1)
	}
	fmt.Printf("%.3f\t", rate.Update(1))
	fmt.Printf("%.3f\t", rate.Update(0))
	fmt.Printf("%.3f\t", rate.Update(0))
	fmt.Printf("%.3f\t", rate.Update(0))
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(1))
	}

	fmt.Println(".")
	fmt.Println("Even more quickly if there's only one failure")
	fmt.Printf("%.3f\t", rate.Update(0))
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(1))
	}

	fmt.Println(".")
	fmt.Println("Unstable dialer should keeps getting a low score")
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(math.Mod(float64(i), 2)))
	}
	// Output:
	//Should recover from hiccups quickly
	//1.000	0.300	0.090	0.027	0.708	0.912	0.974	0.992	0.997	0.999	1.000	1.000	1.000	1.000	.
	//Even more quickly if there's only one failure
	//0.300	0.790	0.937	0.981	0.994	0.998	0.999	1.000	1.000	1.000	1.000	.
	//Unstable dialer should keeps getting a low score
	//0.300	0.790	0.237	0.771	0.231	0.769	0.231	0.769	0.231	0.769
}

func ExampleEMASuccessRateOverlyHigh() {
	rate := ema.New(1, 0.9)
	fmt.Println("Recovers from hiccups quickly")
	for i := 0; i < 100; i++ {
		rate.Update(1)
	}
	fmt.Printf("%.3f\t", rate.Update(1))
	fmt.Printf("%.3f\t", rate.Update(0))
	fmt.Printf("%.3f\t", rate.Update(0))
	fmt.Printf("%.3f\t", rate.Update(0))
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(1))
	}

	fmt.Println(".")
	fmt.Println("Even more quickly if there's only one failure")
	fmt.Printf("%.3f\t", rate.Update(0))
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(1))
	}

	fmt.Println(".")
	fmt.Println("Doesn't perform well for unstable dialer")
	for i := 0; i < 10; i++ {
		fmt.Printf("%.3f\t", rate.Update(math.Mod(float64(i), 2)))
	}
	// Output:
	// Recovers from hiccups quickly
	// 1.000	0.100	0.010	0.001	0.900	0.990	0.999	1.000	1.000	1.000	1.000	1.000	1.000	1.000	.
	// Even more quickly if there's only one failure
	// 0.100	0.910	0.991	0.999	1.000	1.000	1.000	1.000	1.000	1.000	1.000	.
	// Doesn't perform well for unstable dialer
	// 0.100	0.910	0.091	0.909	0.091	0.909	0.091	0.909	0.091	0.909
}
