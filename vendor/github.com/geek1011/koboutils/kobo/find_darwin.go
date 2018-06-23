package kobo

import "fmt"

// bruteForce looks for a kobo by testing the folders starting with /Volumes/KOBOeReader.
func bruteForce() ([]string, error) {
	mounts := []string{"/Volumes/KOBOeReader"}
	for i := 0; i < 5; i++ {
		mounts = append(mounts, fmt.Sprintf("/Volumes/KOBOeReader-%d", i))
	}

	kobos := []string{}
	for _, kobo := range mounts {
		if IsKobo(kobo) {
			kobos = append(kobos, kobo)
		}
	}

	return kobos, nil
}

func init() {
	findFuncs = append(findFuncs, bruteForce)
}
