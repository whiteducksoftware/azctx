package profile

import "fmt"

// ValidateGUID rejects names and malformed IDs before they reach Azure CLI.
func ValidateGUID(id string) error {
	if len(id) != 36 {
		return fmt.Errorf("invalid GUID %q", id)
	}
	for i, char := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if char == '-' {
				continue
			}
		} else if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F' {
			continue
		}
		return fmt.Errorf("invalid GUID %q", id)
	}
	return nil
}
