package main

import "golang.org/x/sys/windows/registry"
import "github.com/pkg/errors"
import "fmt"
import "os/exec"
import "os"
import "encoding/xml"
import "time"
import "strings"

type DSACatalogGranulePick struct {
	XMLName        xml.Name `xml:"GranulePick"`
	PlatformType   string   `xml:"PlatformType,attr"`
	MfgCode        string   `xml:"MfgCode,attr"`
	SelectionState string   `xml:"SelectionState,attr"`
}

type DSACatalogState struct {
	XMLName          xml.Name                `xml:"StateCookieInfo"`
	UsingNetwork     bool                    `xml:"Client>NetworkInfo>IsNetworkDeployment"`
	GranulePicks     []DSACatalogGranulePick `xml:"Client>UserPicks>GranulePicks>GranulePick"`
	LastDiscLocation string
}

const (
	CAP2020_CATALOG          = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\20-20 COMMERCIAL CATALOGS`
	CAP2020_SOFTWARE         = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\{5D4D912A-D5EE-4748-84B8-7C2C75EC4408}`
	CAP2020_SOFTWARE_CURRENT = `13.00.13037`
	PATH_CATALOG             = `\\10.0.9.29\2020catalogbeta\ClientSetup\setup.exe`
	PATH_SOFTWARE            = `\\10.0.9.29\2020software\Setup.exe`
)

const (
	CATALOG_STATE_MISSNG = iota
	CATALOG_STATE_LOCAL
	CATALOG_STATE_NETWORK
	CATALOG_STATE_INVALID
)

func GetCatalogStatus() (int, error) {
	f, err := os.Open(`C:\ProgramData\2020\DSA\2020Catalogs-StateCookie.xml`)
	if err != nil {
		// This is fine, it likely just means the software isn't installed
		return CATALOG_STATE_MISSNG, nil
	}
	defer f.Close()

	var catalogstate DSACatalogState
	dec := xml.NewDecoder(f)
	err = dec.Decode(&catalogstate)
	if err != nil {
		return CATALOG_STATE_INVALID, errors.Wrap(err, "Cannot decode DSA state XML file")
	}

	// The Demo package is mandatory for all installs, so we can check if it's selected
	// in order to determine whether anything is locally installed.
	for j := range catalogstate.GranulePicks {
		if catalogstate.GranulePicks[j].MfgCode == `DMO` &&
			catalogstate.GranulePicks[j].PlatformType == `CAP` &&
			catalogstate.GranulePicks[j].SelectionState == `Selected` {
			return CATALOG_STATE_LOCAL, nil
		}
	}

	if !strings.EqualFold(catalogstate.LastDiscLocation, `\\10.0.9.29\2020catalogbeta\ClientSetup\`) {
		fmt.Printf("Catalog Last Disc Location is incorrectly %s\n", catalogstate.LastDiscLocation)
		return CATALOG_STATE_INVALID, nil
	}

	return CATALOG_STATE_NETWORK, nil
}

func CleanCatalog() error {
	return os.RemoveAll(`C:\ProgramData\2020\DSA`)
}

func UninstallCatalog() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, CAP2020_CATALOG, registry.READ)
	if err != nil {
		return errors.Wrap(err, "Cannot open registry key for uninstall")
	}
	defer k.Close()

	v, _, err := k.GetStringValue("UninstallString")
	if err != nil {
		return errors.Wrap(err, "Cannot read value UninstallString")
	}

	// Verify that the uninstall command looks like one we recognize.
	if !strings.EqualFold(v, `C:\Program Files (x86)\2020\DSA\dsa.exe /removeall /rootpath "C:\ProgramData\2020\DSA"`) {
		return errors.Errorf("UninstallString had an unexpected value of %s", v)
	}

	out, err := exec.Command(`C:\Program Files (x86)\2020\DSA\dsa.exe`, "/removeall", "/rootpath", `"C:\ProgramData\2020\DSA"`).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "Uninstall command output: %s", out)
	}
	return nil
}

// "Is Installed", "Is Current", error
func GetSoftwareStatus() (bool, bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, CAP2020_SOFTWARE, registry.READ)
	if err == registry.ErrNotExist {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Wrap(err, "Cannot open registry key for software version")
	}
	defer k.Close()

	v, _, err := k.GetStringValue("DisplayVersion")
	if err != nil {
		return false, false, errors.Wrap(err, "Cannot read value DisplayVersion")
	}

	return true, (v == CAP2020_SOFTWARE_CURRENT), nil
}

func InstallNetworkCatalog() error {
	out, err := exec.Command(PATH_CATALOG).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "Setup command output: %s", out)
	}

	return nil
}

func InstallSoftware() error {
	out, err := exec.Command(PATH_SOFTWARE).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "Install command output: %s", out)
	}

	return nil
}

func UninstallSoftware() error {
	out, err := exec.Command("msiexec", "/x", `{5D4D912A-D5EE-4748-84B8-7C2C75EC4408}`, "/passive", "/forcerestart").CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "Uninstall command output: %s", out)
	}

	return nil
}

func ExitWithSuccess(m string) {
	fmt.Printf("SUCCESS: %s\n\n", m)
	time.Sleep(10 * time.Second)
	os.Exit(0)
}

func ExitWithError(m string, e error) {
	fmt.Printf("ERROR: %s (%+v)\n\n", m, e)
	time.Sleep(5 * time.Minute)
	os.Exit(1)
}

func ExitWithoutSuccess(m string) {
	fmt.Printf("UNSUCCESSFUL: %s\n\n", m)
	time.Sleep(5 * time.Minute)
	os.Exit(2)
}

func main() {
	var err error

	softInstalled, softCurrent, err := GetSoftwareStatus()
	if err != nil {
		ExitWithError("Unable to check software status.", err)
	}

	if !softInstalled {
		fmt.Println("2020 software is not installed.")
		err = InstallSoftware()
		if err != nil {
			ExitWithError("Unable to install the 2020 software. Restart your computer and try again manually.", err)
		}
		ExitWithoutSuccess("Complete the install process manually and run this again afterward.")
	}

	if !softCurrent {
		fmt.Println("2020 software is out of date. Uninstalling current software...")
		err = UninstallSoftware()
		if err != nil {
			ExitWithError("Unable to uninstall the 2020 software. Restart your computer and try again manually.", err)
		}
		ExitWithoutSuccess("Software uninstall will require a reboot. After reboot, run again to update software.")
	}

	fmt.Println("Looks like the 2020 software is up to date. Let's check your catalog...")

	catState, err := GetCatalogStatus()
	if err != nil {
		ExitWithError("Unable to check for Network Deployment.", err)
	}

	if catState == CATALOG_STATE_NETWORK {
		ExitWithSuccess("You are using the 2020 Network Deployment. Nice.")
	}

	if catState == CATALOG_STATE_LOCAL {
		fmt.Println("Looks like you have the catalog installed locally, not on the network.")
		fmt.Println("Uninstalling local catalog.")
		err = UninstallCatalog()
		if err != nil {
			ExitWithError("Can't run the uninstaller for the catalog. Try running it yourself.", err)
		}
		fmt.Println("Clearing out remaining files after uninstall.")
		CleanCatalog()
	}

	fmt.Println("Installing the network catalog...")
	err = InstallNetworkCatalog()
	if err != nil {
		ExitWithError("Failed to install the network catalog.", err)
	}
	fmt.Println("Checking the catalog status again...")
	catState, err = GetCatalogStatus()
	if err == nil && catState == CATALOG_STATE_NETWORK {
		ExitWithSuccess("Looks good. Network catalog is now installed.")
	}
	ExitWithoutSuccess("Finish installing the catalog by using the wizard. You can close this window.")
}
