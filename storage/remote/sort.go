package remote

import (
	"github.com/prometheus/prometheus/prompb"
)

func sortSamples(arr *[]prompb.Sample, startIndex, endIndex int) int {
	if startIndex >= endIndex {
		return -1
	}
	var pivotIndex = partition(arr, startIndex, endIndex)
	sortSamples(arr, startIndex, pivotIndex-1)
	sortSamples(arr, pivotIndex+1, endIndex)
	return 0
}

func partition(arr0 *[]prompb.Sample, startIndex, endIndex int) int {
	arr := *arr0
	var pivot = arr[startIndex].Timestamp
	var left = startIndex
	var right = endIndex
	for left != right {
		for left < right && arr[right].Timestamp > pivot {
			right--
		}

		for left < right && arr[left].Timestamp <= pivot {
			left++
		}
		if left < right {
			p := arr[left]
			arr[left] = arr[right]
			arr[right] = p
		}
	}
	p := arr[left]
	arr[left] = arr[startIndex]
	arr[startIndex] = p
	return left
}
