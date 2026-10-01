package toolsearch

import "sort"

func sortMatches(matches []Match) {
	sort.Slice(matches, func(first, second int) bool {
		if matches[first].Similarity != matches[second].Similarity {
			return matches[first].Similarity > matches[second].Similarity
		}
		return matches[first].ID < matches[second].ID
	})
}
