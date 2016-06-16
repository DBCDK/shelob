package main

import "strings"

func reverseStringArray(array []string) []string {
	for i, j := 0, len(array)-1; i < j; i, j = i+1, j-1 {
		array[i], array[j] = array[j], array[i]
	}

	return array
}

func stripPortFromDomain(domainWithPort string) string {
	if strings.Contains(domainWithPort, ":") {
		domainWithoutPort := strings.Split(domainWithPort, ":")[0]
		return domainWithoutPort
	}

	return domainWithPort
}
