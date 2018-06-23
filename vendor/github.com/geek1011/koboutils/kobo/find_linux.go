package kobo

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"os/user"
	"strings"
)

// findmnt uses the findmnt command to look for a kobo.
func findmnt() ([]string, error) {
	findmnt, err := exec.LookPath("findmnt")
	if err != nil {
		return nil, ErrCommandNotFound
	}

	buf := bytes.NewBuffer(nil)

	cmd := exec.Command(findmnt, "-nlo", "TARGET", "LABEL=KOBOeReader")
	cmd.Stderr = ioutil.Discard
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		if !strings.Contains(err.Error(), "status 1") {
			return nil, err
		}
	}

	kobos := []string{}
	for _, kobo := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if IsKobo(kobo) {
			kobos = append(kobos, kobo)
		}
	}
	return kobos, nil
}

// mediamnt checks for a kobo at /media/USERNAME/KOBOeReader and /run/media/USERNAME/KOBOeReader.
func mediamnt() ([]string, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	kobos := []string{}
	for _, kobo := range []string{
		fmt.Sprintf("/media/%s/KOBOeReader", u.Username),
		fmt.Sprintf("/run/media/%s/KOBOeReader", u.Username),
	} {
		if IsKobo(kobo) {
			kobos = append(kobos, kobo)
		}
	}
	return kobos, nil
}

func init() {
	findFuncs = append(findFuncs, findmnt, mediamnt)
}
