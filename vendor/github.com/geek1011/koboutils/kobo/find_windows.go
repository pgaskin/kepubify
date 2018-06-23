package kobo

// bruteForce looks for a kobo by testing the drive letters backwards from Z-A.
func bruteForce() ([]string, error) {
	kobos := []string{}
	letters := []string{"Z", "Y", "X", "W", "V", "U", "T", "S", "R", "Q", "P", "O", "N", "M", "L", "K", "J", "I", "H", "G", "F", "E", "D", "C", "B", "A"}
	for _, letter := range letters {
		kobo := letter + ":"
		if IsKobo(kobo) {
			kobos = append(kobos, kobo)
		}
	}
	return kobos, nil
}

func init() {
	findFuncs = append(findFuncs, bruteForce)
}
