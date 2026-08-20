package redis

import "strconv"

// StreamNameForNamespace returns the Redis stream assigned to a namespace.
// The hash matches the signed JavaScript implementation used by the Socket.IO
// Redis Streams adapter, including its UTF-16 code-unit handling.
func StreamNameForNamespace(streamName, namespaceName string, streamCount int) string {
	if streamCount <= 1 {
		return streamName
	}

	index := int64(streamHashCode(namespaceName)) % int64(streamCount)
	return streamName + "-" + strconv.FormatInt(index, 10)
}

func streamHashCode(value string) int32 {
	var hash int32
	for _, codePoint := range value {
		if codePoint <= 0xffff {
			hash = hash*31 + codePoint
			continue
		}

		codePoint -= 0x10000
		hash = hash*31 + 0xd800 + (codePoint >> 10)
		hash = hash*31 + 0xdc00 + (codePoint & 0x3ff)
	}
	return hash
}
