package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	promVersion "github.com/prometheus/common/version"

	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/vehicle/bluelink"

	"github.com/samber/lo"
)

func init() {
	promVersion.Version = "0.1.0"
	prometheus.MustRegister(version.NewCollector("ioniq_exporter"))
}

func main() {
	var (
		listenAddr   = flag.String("web.listen-address", ":9333", "The address to listen on for HTTP requests.")
		username     = flag.String("username", "user@example.com", "Login email for Hyundai Bluelink")
		token        = flag.String("token", "secret1234", "Refresh token for Hyundai Bluelink account")
		vin          = flag.String("vin", "TM...", "VIN of vehicle to check")
		pollInterval = flag.Int("poll-interval", 60, "Interval in seconds between polls.")
		showVersion  = flag.Bool("version", false, "Print version information and exit.")
	)

	flag.Parse()

	if *showVersion {
		fmt.Printf("%s\n", promVersion.Print("ioniq_exporter"))
		os.Exit(0)
	}

	var evRange = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ev_range",
		Help: "Electric vehicle range",
	})
	var evSoc = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ev_soc",
		Help: "Electric vehicle state of charge",
	})
	var evStatus = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ev_status",
		Help: "Electric vehicle status",
	})
	var evFinishTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ev_finish_time",
		Help: "Electric charging finish time",
	})
	var evOdometer = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ev_odometer",
		Help: "Electric odometer",
	})

	// Register the summary and the histogram with Prometheus's default registry
	prometheus.MustRegister(evRange)
	prometheus.MustRegister(evSoc)
	prometheus.MustRegister(evStatus)
	prometheus.MustRegister(evFinishTime)
	prometheus.MustRegister(evOdometer)

	// Add Go module build info
	prometheus.MustRegister(collectors.NewBuildInfoCollector())

	// Use connect credentials to build provider
	// Check https://github.com/evcc-io/evcc/blob/master/vehicle/bluelink.go#L28-L37 for updates
	settings := bluelink.Config{
		URI:               "https://prd.eu-ccapi.hyundai.com:8080",
		BasicToken:        "NmQ0NzdjMzgtM2NhNC00Y2YzLTk1NTctMmExOTI5YTk0NjU0OktVeTQ5WHhQekxwTHVvSzB4aEJDNzdXNlZYaG10UVI5aVFobUlGampvWTRJcHhzVg==",
		CCSPServiceID:     "6d477c38-3ca4-4cf3-9557-2a1929a94654",
		CCSPApplicationID: bluelink.HyundaiAppID,
		AuthClientID:      "6d477c38-3ca4-4cf3-9557-2a1929a94654",
		BrandAuthUrl:      "%s/auth/api/v2/user/oauth2/authorize?response_type=code&client_id=%s&redirect_uri=%s/api/v1/user/oauth2/redirect&lang=%s&state=ccsp",
		PushType:          "GCM",
		Cfb:               "RFtoRq/vDXJmRndoZaZQyfOot7OrIqGVFj96iY2WL3yyH5Z/pUvlUhqmCxD2t+D65SQ=",
		LoginFormHost:     "https://idpconnect-eu.hyundai.com",
	}

	logHandler := util.NewLogger("ioniq").Redact(*username, *token, *vin)

	// Poll inverter values
	go func() {
		for {
			identity := bluelink.NewIdentity(logHandler, settings)
			if err := identity.Login(*username, *token, "en", "hyundai"); err != nil {
				log.Println(err)
				time.Sleep(time.Duration(*pollInterval) * time.Second)
				continue
			}

			api := bluelink.NewAPI(logHandler, settings.URI, identity.Request)

			vehicle, err := ensureVehicleEx(*vin, api.Vehicles, func(v bluelink.Vehicle) (string, error) {
				return v.VIN, nil
			},
			)
			if err != nil {
				log.Println(err)
				time.Sleep(time.Duration(*pollInterval) * time.Second)
				continue
			}

			provider := bluelink.NewProvider(api, vehicle, time.Second*30, time.Second*30)

			rangeKm, err := provider.Range()
			if err != nil {
				log.Print("Range Error: ", err)
			} else {
				evRange.Set(float64(rangeKm))
			}

			soc, err := provider.Soc()
			if err != nil {
				log.Print("SoC error: ", err)
			} else {
				evSoc.Set(soc)
			}

			statusString, err := provider.Status()
			if err != nil {
				log.Print("Status error: ", err)
			} else {
				switch statusString.String() {
				case "":
					evStatus.Set(0)
				case "A":
					evStatus.Set(1)
				case "B":
					evStatus.Set(2)
				case "C":
					evStatus.Set(3)
				case "D":
					evStatus.Set(4)
				case "E":
					evStatus.Set(5)
				case "F":
					evStatus.Set(6)
				default:
					log.Print("Unknown status: ", statusString)
				}
			}

			finishTime, err := provider.FinishTime()
			if err != nil {
				log.Print("Finish time error: ", err)
			} else {
				evFinishTime.Set(float64(finishTime.Unix()))
			}

			odometer, err := provider.Odometer()
			if err != nil {
				log.Print("Odometer error: ", err)
			} else {
				evOdometer.Set(odometer)
			}

			time.Sleep(time.Duration(*pollInterval) * time.Second)
		}
	}()

	// Expose the registered metrics via HTTP
	http.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{},
	))
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

// ensureVehicleEx extracts vehicle with matching VIN from list of vehicles
// Copied from https://github.com/evcc-io/evcc/blob/master/vehicle/helper.go
func ensureVehicleEx[T any](
	vin string,
	list func() ([]T, error),
	extract func(T) (string, error),
) (T, error) {
	var zero T

	vehicles, err := list()
	if err != nil {
		return zero, fmt.Errorf("cannot get vehicles: %w", err)
	}

	if vin := strings.ToUpper(vin); vin != "" {
		// vin defined
		for _, vehicle := range vehicles {
			vv, err := extract(vehicle)
			if err != nil {
				return zero, err
			}
			if strings.ToUpper(vv) == vin {
				return vehicle, nil
			}
		}
	} else if len(vehicles) == 1 {
		// vin empty and exactly one vehicle
		return vehicles[0], nil
	}

	return zero, fmt.Errorf("cannot find vehicle, got: %v", lo.Map(vehicles, func(v T, _ int) string {
		vin, _ := extract(v)
		return vin
	}))
}
