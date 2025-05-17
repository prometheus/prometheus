// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build toplabels

package labels

import (
	"slices"
	"unsafe"
)

var (
	// List of labels that should be mapped to a single byte value.
	// Obviously can't have more than 256 here.
	mappedLabels     = []string{}
	mappedLabelIndex = map[string]byte{}
)

// MapLabels takes a list of strings that shuld use a single byte storage
// inside labels, making them use as little memory as possible.
// Since we use a single byte mapping we can only have 256 such strings.
//
// We MUST store empty string ("") as one of the values here and if you
// don't pass it into MapLabels() then it will be injected.
//
// If you pass more strings than 256 then extra strings will be ignored.
func MapLabels(names []string) {
	// We must always store empty string. Push it to the front of the slice if not present.
	if !slices.Contains(names, "") {
		names = append([]string{""}, names...)
	}

	mappedLabels = make([]string, 0, 256)
	mappedLabelIndex = make(map[string]byte, 256)

	for i, name := range names {
		if i >= 256 {
			break
		}
		mappedLabels = append(mappedLabels, name)
		mappedLabelIndex[name] = byte(i)
	}
}

func init() {
	names := []string{
		// Empty string, this must be present here.
		"",
		// These label names are always present on every time series.
		MetricName,
		InstanceName,
		"job",
		// Common label names.
		BucketLabel,
		"code",
		"handler",
		"quantile",
		// Meta metric names injected by Prometheus itself.
		"scrape_body_size_bytes",
		"scrape_duration_seconds",
		"scrape_sample_limit",
		"scrape_samples_post_metric_relabeling",
		"scrape_samples_scraped",
		"scrape_series_added",
		"scrape_timeout_seconds",
		// Common metric names from client libraries.
		"process_cpu_seconds_total",
		"process_max_fds",
		"process_network_receive_bytes_total",
		"process_network_transmit_bytes_total",
		"process_open_fds",
		"process_resident_memory_bytes",
		"process_start_time_seconds ",
		"process_virtual_memory_bytes",
		"process_virtual_memory_max_bytes",
		// client_go specific metrics
		"go_gc_heap_frees_by_size_bytes_bucket",
		"go_gc_heap_allocs_by_size_bytes_bucket",
		"net_conntrack_dialer_conn_failed_total",
		"go_sched_pauses_total_other_seconds_bucket",
		"go_sched_pauses_total_gc_seconds_bucket",
		"go_sched_pauses_stopping_other_seconds_bucket",
		"go_sched_pauses_stopping_gc_seconds_bucket",
		"go_sched_latencies_seconds_bucket",
		"go_gc_pauses_seconds_bucket",
		"go_gc_duration_seconds",
		// node_exporter metrics
		"node_cpu_seconds_total",
		"node_scrape_collector_success",
		"node_scrape_collector_duration_seconds",
		"node_cpu_scaling_governor",
		"node_cpu_guest_seconds_total",
		"node_hwmon_temp_celsius",
		"node_hwmon_sensor_label",
		"node_hwmon_temp_max_celsius",
		"node_cooling_device_max_state",
		"node_cooling_device_cur_state",
		"node_softnet_times_squeezed_total",
		"node_softnet_received_rps_total",
		"node_softnet_processed_total",
		"node_softnet_flow_limit_count_total",
		"node_softnet_dropped_total",
		"node_softnet_cpu_collision_total",
		"node_softnet_backlog_len",
		"node_schedstat_waiting_seconds_total",
		"node_schedstat_timeslices_total",
		"node_schedstat_running_seconds_total",
		"node_cpu_scaling_frequency_min_hertz",
		"node_cpu_scaling_frequency_max_hertz",
		"node_cpu_scaling_frequency_hertz",
		"node_cpu_frequency_min_hertz",
		"node_cpu_frequency_max_hertz",
		"node_hwmon_temp_crit_celsius",
		"node_hwmon_temp_crit_alarm_celsius",
		"node_cpu_core_throttles_total",
		"node_thermal_zone_temp",
		"node_hwmon_temp_min_celsius",
		"node_hwmon_chip_names",
		"node_filesystem_readonly",
		"node_filesystem_device_error",
		"node_filesystem_size_bytes",
		"node_filesystem_free_bytes",
		"node_filesystem_files_free",
		"node_filesystem_files",
		"node_filesystem_avail_bytes",
	}
	MapLabels(names)
}

func decodeSize(data string, index int) (int, int, bool) {
	b := data[index]
	index++
	if b == 0 {
		// 0 means that the label was mapped.
		return 1, index, true
	}
	if b == 255 {
		// Larger numbers are encoded as 3 bytes little-endian.
		// Just panic if we go of the end of data, since all Labels strings are constructed internally and
		// malformed data indicates a bug, or memory corruption.
		return int(data[index]) + (int(data[index+1]) << 8) + (int(data[index+2]) << 16), index + 3, false
	}
	// More common case of a single byte, value 1..254.
	return int(b), index, false
}

func decodeString(data string, index int) (string, int) {
	var size int
	var mapped bool
	size, index, mapped = decodeSize(data, index)
	if mapped {
		b := data[index]
		return mappedLabels[int(b)], index + size
	}
	return data[index : index+size], index + size
}

func encodeShortString(s string) (int, byte) {
	if i, ok := mappedLabelIndex[s]; ok {
		return 0, i
	}
	return len(s), 0
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	if name == "" { // Avoid crash in loop if someone asks for "".
		return "" // Prometheus does not store blank label names.
	}
	for i := 0; i < len(ls.data); {
		var size, next int
		var mapped bool
		var lName, lValue string
		size, next, mapped = decodeSize(ls.data, i) // Read the key index and size.
		if mapped {                                 // Key is a mapped string, so decode it fully and move i to the value index.
			lName, i = decodeString(ls.data, i)
			if lName == name {
				lValue, _ = decodeString(ls.data, i)
				return lValue
			}
			if lName[0] > name[0] { // Stop looking if we've gone past.
				break
			}
		} else { // Value is stored raw in the data string.
			i = next // Move index to the start of the key string.
			if ls.data[i] == name[0] {
				lName = ls.data[i : i+size]
				i += size // We got the key string, move the index to the start of the value.
				if lName == name {
					lValue, _ := decodeString(ls.data, i)
					return lValue
				}
			} else {
				if ls.data[i] > name[0] { // Stop looking if we've gone past.
					break
				}
				i += size
			}
		}
		size, i, _ = decodeSize(ls.data, i) // Read the value index and size.
		i += size                           // move the index past the value so we can read the next key.
	}
	return ""
}

// Has returns true if the label with the given name is present.
func (ls Labels) Has(name string) bool {
	if name == "" { // Avoid crash in loop if someone asks for "".
		return false // Prometheus does not store blank label names.
	}
	for i := 0; i < len(ls.data); {
		var size, next int
		var mapped bool
		var lName string
		size, next, mapped = decodeSize(ls.data, i)
		if mapped {
			lName, i = decodeString(ls.data, i)
			if lName == name {
				return true
			}
			if lName[0] > name[0] { // Stop looking if we've gone past.
				break
			}
		} else {
			i = next
			if ls.data[i] == name[0] {
				lName = ls.data[i : i+size]
				if lName == name {
					return true
				}
			} else {
				if ls.data[i] > name[0] { // Stop looking if we've gone past.
					break
				}
			}
			i += size
		}
		size, i, _ = decodeSize(ls.data, i)
		i += size
	}
	return false
}

// Compare compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func Compare(a, b Labels) int {
	// Find the first byte in the string where a and b differ.
	shorter, longer := a.data, b.data
	if len(b.data) < len(a.data) {
		shorter, longer = b.data, a.data
	}
	i := 0
	// First, go 8 bytes at a time. Data strings are expected to be 8-byte aligned.
	sp := unsafe.Pointer(unsafe.StringData(shorter))
	lp := unsafe.Pointer(unsafe.StringData(longer))
	for ; i < len(shorter)-8; i += 8 {
		if *(*uint64)(unsafe.Add(sp, i)) != *(*uint64)(unsafe.Add(lp, i)) {
			break
		}
	}
	// Now go 1 byte at a time.
	for ; i < len(shorter); i++ {
		if shorter[i] != longer[i] {
			break
		}
	}
	if i == len(shorter) {
		// One Labels was a prefix of the other; the set with fewer labels compares lower.
		return len(a.data) - len(b.data)
	}

	// Now we know that there is some difference before the end of a and b.
	// Go back through the fields and find which field that difference is in.
	firstCharDifferent, i := i, 0
	size, nextI, _ := decodeSize(a.data, i)
	for nextI+size <= firstCharDifferent {
		i = nextI + size
		size, nextI, _ = decodeSize(a.data, i)
	}
	// Difference is inside this entry.
	aStr, _ := decodeString(a.data, i)
	bStr, _ := decodeString(b.data, i)
	if aStr < bStr {
		return -1
	}
	return +1
}

// Len returns the number of labels; it is relatively slow.
func (ls Labels) Len() int {
	count := 0
	for i := 0; i < len(ls.data); {
		var size int
		size, i, _ = decodeSize(ls.data, i)
		i += size
		size, i, _ = decodeSize(ls.data, i)
		i += size
		count++
	}
	return count
}

// DropMetricName returns Labels with "__name__" removed.
func (ls Labels) DropMetricName() Labels {
	for i := 0; i < len(ls.data); {
		lName, i2 := decodeString(ls.data, i)
		size, i2, _ := decodeSize(ls.data, i2)
		i2 += size
		if lName == MetricName {
			if i == 0 { // Make common case fast with no allocations.
				ls.data = ls.data[i2:]
			} else {
				ls.data = ls.data[:i] + ls.data[i2:]
			}
			break
		} else if lName[0] > MetricName[0] { // Stop looking if we've gone past.
			break
		}
		i = i2
	}
	return ls
}

func marshalLabelToSizedBuffer(m *Label, data []byte) int {
	i := len(data)

	size, b := encodeShortString(m.Value)
	if size == 0 {
		i--
		data[i] = b
	} else {
		i -= size
		copy(data[i:], m.Value)
	}
	i = encodeSize(data, i, size)

	size, b = encodeShortString(m.Name)
	if size == 0 {
		i--
		data[i] = b
	} else {
		i -= size
		copy(data[i:], m.Name)
	}
	i = encodeSize(data, i, size)

	return len(data) - i
}

func labelSize(m *Label) (n int) {
	// strings are encoded as length followed by contents.
	l, _ := encodeShortString(m.Name)
	if l == 0 {
		l++
	}
	n += l + sizeWhenEncoded(uint64(l))

	l, _ = encodeShortString(m.Value)
	if l == 0 {
		l++
	}
	n += l + sizeWhenEncoded(uint64(l))
	return n
}
