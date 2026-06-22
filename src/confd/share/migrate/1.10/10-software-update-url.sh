#!/bin/sh
# Move software/check-update/update-url to the shared software/update-url, and
# convert it from a repository URL to an RSS/Atom release feed.
#
# The update source was lifted out of the check-update container so that
# check-update and unattended-update share a single setting, and the latest
# version is now read from a release feed instead of the GitHub REST API.
# Configs that never set it are left untouched; the new default already names
# the feed.

file=$1
temp=${file}.tmp

jq '
    ["ietf-system:system", "infix-system:software", "check-update", "update-url"] as $old
  | ["ietf-system:system", "infix-system:software", "update-url"]                as $new
  | (if getpath($old) != null
     then setpath($new; getpath($old)) | delpaths([$old])
     else . end)
  | (getpath($new) as $url
     | if ($url | type) == "string" and (($url | endswith(".atom")) | not)
       then setpath($new; ($url | sub("/+$"; "")) + "/releases.atom")
       else . end)
' "$file" > "$temp" && mv "$temp" "$file"
