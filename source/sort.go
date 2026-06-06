package source

import "sort"

// SortSeriesList orders series by SeriesInstanceUID.
func SortSeriesList(seriesList []SeriesInfo) {
	sort.SliceStable(seriesList, func(i, j int) bool {
		return seriesList[i].SeriesInstanceUID < seriesList[j].SeriesInstanceUID
	})
}

// SortInstanceInfos orders instances by InstanceNumber when available, then by
// SOPInstanceUID.
func SortInstanceInfos(instances []InstanceInfo) {
	sort.SliceStable(instances, func(i, j int) bool {
		left := instances[i]
		right := instances[j]

		if left.HasInstanceNumber && right.HasInstanceNumber {
			if left.InstanceNumber != right.InstanceNumber {
				return left.InstanceNumber < right.InstanceNumber
			}
		}

		if left.HasInstanceNumber != right.HasInstanceNumber {
			return left.HasInstanceNumber
		}

		return left.Ref.SOPInstanceUID < right.Ref.SOPInstanceUID
	})
}
