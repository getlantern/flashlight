package keyman

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	// TODO: Make sure to handle case where library is on a different path
	OSX_SYSTEM_KEYCHAIN_PATH = "/Library/Keychains/System.keychain"
)

func DeleteTrustedRootByName(commonName string, prompt string) error {
	cmd := elevatedIfNecessary(prompt)("security", "delete-certificate", "-c", commonName, OSX_SYSTEM_KEYCHAIN_PATH)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Unable to run security command: %w\n%s", err, out)
	}
	return nil
}

// isInstalled checks whether this certificate is install based purely on looking for a cert
// in the system keychain that has the same common name.  This function returns
// true if there are one or more certs in the system keychain whose common name
// matches this cert.
func (cert *Certificate) isInstalled() bool {
	cmd := exec.Command("security", "find-certificate", "-c", cert.X509().Subject.CommonName, OSX_SYSTEM_KEYCHAIN_PATH)
	err := cmd.Run()

	return err == nil
}

// AddAsTrustedRootIfNeeded adds the certificate to the user's trust store as a trusted
// root CA.
// elevatePrompt will be displayed when asking for admin permissions
// installPromptTitle/Content are ignored
// If installAttempted is provided it will be called on any attempt to modify system cert store with the resulting
// error (if any)
func (cert *Certificate) AddAsTrustedRootIfNeeded(elevatePrompt, installPromptTitle, installPromptContent string, installAttempted func(error)) error {
	if cert.isInstalled() {
		return nil
	}

	reportInstallResult := func(err error) error {
		if installAttempted != nil {
			installAttempted(err)
		}
		return err
	}

	tempFileName, err := cert.WriteToTempFile()
	defer func() {
		if err := os.Remove(tempFileName); err != nil {
			log.Debugf("Unable to remove file: %w", err)
		}
	}()
	if err != nil {
		return reportInstallResult(err)
	}

	cmd := exec.Command("security", "verify-cert", "-c", tempFileName)
	_, err = cmd.CombinedOutput()
	if err == nil {
		// certificate verified successfully so it's already a trusted root, no need
		// to install.
		return nil
	}

	// Add it as a trusted cert
	cmd = elevatedIfNecessary(elevatePrompt)("security", "add-trusted-cert", "-d", "-k", OSX_SYSTEM_KEYCHAIN_PATH, tempFileName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("Unable to run security command: %w\n%s", err, out)
	}
	reportInstallResult(err)
	if err != nil {
		return err
	}

	cmd = exec.Command("security", "verify-cert", "-c", tempFileName)
	out, err = cmd.CombinedOutput()
	log.Debugf("%v: %v", out, err)
	return nil
}
