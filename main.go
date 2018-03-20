package main

import (
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"tracy/api/common"
	"tracy/api/rest"
	"tracy/api/store"
	"tracy/api/types"
	"tracy/configure"
	"tracy/install"
	"tracy/log"
	"tracy/proxy"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	if *cpuprofile != "" {
		defer pprof.StopCPUProfile()
	}
	/* Start the proxy. */
	go func() {
		/* Open a TCP listener. */
		ln := configure.ProxyServer()

		/* TODO: move to the init stuff, this isn't really a configuration. Load the configured certificates. */
		configure.Certificates()

		/* Serve it. This will block until the user closes the program. */
		proxy.ListenAndServe(ln)
	}()
	ps, err := configure.ReadConfig("proxy-server")
	if err != nil {
		log.Error.Fatal(err)
	}
	log.PrintCyan(fmt.Sprintf("Proxy server:\t%s\n", ps.(string)))

	/* Serve it. Block here so the program doesn't close. */
	go func() {
		log.Error.Fatal(rest.ConfigServer.ListenAndServe())
	}()
	log.PrintCyan(fmt.Sprintf("Config server:\t%s\n", "127.0.0.1:6666"))

	/* Serve it. Block here so the program doesn't close. */
	go func() {
		log.Error.Fatal(rest.RestServer.ListenAndServe())
	}()
	ts, err := configure.ReadConfig("tracer-server")
	if err != nil {
		log.Error.Fatal(err)
	}
	log.PrintCyan(fmt.Sprintf("Tracer server:\t%s\n", ts.(string)))

	autoLaunch, err := configure.ReadConfig("auto-launch")
	if err != nil {
		log.Error.Fatal(err.Error())
	}

	processAutoLaunch(autoLaunch.(string))

	/* Waiting for the user to close the program. */
	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		for _ = range signalChan {
			fmt.Println("Ctrl+C pressed. Shutting down...")
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}

func init() {
	// Parse the flags. Have to parse them hear since other package initialize command line
	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
	}

	// Set up the logging based on the user command line flags
	log.Configure()
	// Open the database
	if err := store.Open(configure.DatabaseFile, log.Verbose); err != nil {
		log.Error.Fatal(err.Error())
	}

	// Initialize the rest routes
	rest.Configure()

	//TODO: decide if we want to add labels to the database or just keep in them in a configuration file
	/* Add the configured labels to the database. */
	tracers, err := configure.ReadConfig("tracers")
	if err != nil {
		log.Error.Fatal(err.Error())
	}
	for k, v := range tracers.(map[string]interface{}) {
		label := types.Label{
			TracerString:  k,
			TracerPayload: v.(string),
		}

		common.AddLabel(label)
	}

	// Instantiate the certificate cache
	certsJSON, err := ioutil.ReadFile(configure.CertCacheFile)
	if err != nil {
		certsJSON = []byte("[]")
		// Can recover from this. Simply make a cache file and instantiate an empty cache.
		ioutil.WriteFile(configure.CertCacheFile, certsJSON, os.ModePerm)
	}

	certs := []proxy.CertCacheEntry{}
	if err := json.Unmarshal(certsJSON, &certs); err == nil {
		cache := make(map[string]tls.Certificate)
		for _, cert := range certs {
			keyPEM := cert.Certs.KeyPEM
			certPEM := cert.Certs.CertPEM

			cachedCert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certPEM}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyPEM}))
			if err != nil {
				log.Error.Println(err)
			}

			cache[cert.Host] = cachedCert
		}

		proxy.SetCertCache(cache)
	} else {
		log.Error.Println(err)
		log.Error.Println(string(certsJSON))
	}

	path, err := configure.ReadConfig("installation-path")
	if err != nil {
		log.Error.Fatal(err)
	}
	if _, err := os.Stat(path.(string)); path.(string) == "" || os.IsNotExist(err) {
		// Looks like the installation-path has not been set or is outdated.
		getSideloadLocation()
	}
}

func processAutoLaunch(option string) {
	switch option {
	case "default":
		openbrowser("http://localhost:8081")
	case "off":
		return
	default:
		var cmd *exec.Cmd
		optionArray := strings.Split(option, " ")
		if len(optionArray) == 1 {
			cmd = exec.Command(optionArray[0])
		} else if len(optionArray) > 1 {
			cmd = exec.Command(optionArray[0], optionArray[1:]...)
		} else {
			return
		}
		cmd.Run()
	}
}

//Taken from here https://gist.github.com/hyg/9c4afcd91fe24316cbf0
func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Error.Fatal(err)
	}
}

//Helper function to ask the user where to sideload the extension.
func getSideloadLocation() {
	for {
		browser := install.Input("Which browser are you using? Firefox or Chrome (F/c): ")
		switch strings.ToLower(string(browser[0])) {
		case "f", "\n":
			install.Firefox()
			return
		case "c":
			//install.Chrome()
			log.PrintRed("Chrome is currently not supported.\n")
		default:
			log.PrintRed("Unsupported browser choice.\n")
		}
	}
}
