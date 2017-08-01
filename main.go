package main

import "golang.org/x/sys/windows/registry"
import "github.com/pkg/errors"
import "fmt"
import "os/exec"
import "os"
import "encoding/xml"
import "time"

type DSACatalogGranulePick struct {
	XMLName        xml.Name `xml:"GranulePick"`
	PlatformType   string   `xml:"PlatformType,attr"`
	MfgCode        string   `xml:"MfgCode,attr"`
	SelectionState string   `xml:"SelectionState,attr"`
}

type DSACatalogState struct {
	XMLName      xml.Name                `xml:"StateCookieInfo"`
	UsingNetwork bool                    `xml:"Client>NetworkInfo>IsNetworkDeployment"`
	GranulePicks []DSACatalogGranulePick `xml:"Client>UserPicks>GranulePicks>GranulePick"`
}

const (
	CAP2020_CATALOG          = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\20-20 COMMERCIAL CATALOGS`
	CAP2020_SOFTWARE         = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\{5D4D912A-D5EE-4748-84B8-7C2C75EC4408}`
	CAP2020_SOFTWARE_CURRENT = `13.00.13037`
	PATH_CATALOG             = `\\10.0.9.29\2020catalogbeta`
	PATH_SOFTWARE            = `\\10.0.9.29\2020software\Setup.exe`
)

// Returned tuple is "installed", "on network", "error"
func GetCatalogStatus() (bool, bool, error) {
	f, err := os.Open(`C:\ProgramData\2020\DSA\2020Catalogs-StateCookie.xml`)
	if err == os.ErrNotExist {
		// This is fine, it just means the software isn't installed
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Wrap(err, "Cannot open DSA state XML file")
	}
	defer f.Close()

	var catalogstate DSACatalogState
	dec := xml.NewDecoder(f)
	err = dec.Decode(&catalogstate)
	if err != nil {
		return false, false, errors.Wrap(err, "Cannot decode DSA state XML file")
	}

	// The Demo package is mandatory for all installs, so we can check if it's selected
	// in order to determine whether anything is locally installed.
	for j := range catalogstate.GranulePicks {
		if catalogstate.GranulePicks[j].MfgCode == `DMO` &&
			catalogstate.GranulePicks[j].PlatformType == `CAP` &&
			catalogstate.GranulePicks[j].SelectionState == `Selected` {
			return true, catalogstate.UsingNetwork, nil
		}
	}

	return false, catalogstate.UsingNetwork, nil
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
	if v != `C:\Program Files (x86)\2020\DSA\dsa.exe /removeall /rootpath "C:\ProgramData\2020\DSA"` {
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
	exec.Command("net", "use", "A:", "/delete").Run()

	out, err := exec.Command("net", "use", "A:", PATH_CATALOG, "/persistent:no").CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "NET USE command output: %s", out)
	}

	out, err = exec.Command(`A:\ClientSetup\setup.exe`).CombinedOutput()
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

	catInstalled, catOnNetwork, err := GetCatalogStatus()
	if err != nil {
		ExitWithError("Unable to check for Network Deployment.", err)
	}

	if catOnNetwork {
		ExitWithSuccess("You are using the 2020 Network Deployment. Nice.")
		return
	}

	if catInstalled && !catOnNetwork {
		fmt.Println("Looks like you have the catalog installed locally, not on the network.")
		err = UninstallCatalog()
		if err != nil {
			ExitWithError("Can't run the uninstaller for the catalog. Try running it yourself.", err)
		}
		fmt.Println("Checking the catalog status again...")
		catInstalled, catOnNetwork, err = GetCatalogStatus()
		if (err != nil) || (catInstalled && !catOnNetwork) {
			ExitWithoutSuccess("Finish uninstalling the local catalog, then run this again. You can close this window.")
		}
	}

	fmt.Println("Installing the network catalog...")
	err = InstallNetworkCatalog()
	if err != nil {
		ExitWithError("Failed to install the network catalog.", err)
	}
	fmt.Println("Checking the catalog status again...")
	catInstalled, catOnNetwork, err = GetCatalogStatus()
	if err == nil && catInstalled && catOnNetwork {
		ExitWithSuccess("Looks good. Network catalog is installed.")
	}
	ExitWithoutSuccess("Finish installing the catalog by using the wizard. You can close this window.")
}
