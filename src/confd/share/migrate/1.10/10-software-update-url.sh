#!/bin/sh
# Move software/check-update/update-url to the shared software/update-url.
#
# The update-source URL was lifted out of the check-update container so that
# check-update and unattended-update share a single setting.  Relocate any
# configured value to the new location and drop the old leaf; configs that
# never set it are left untouched.

file=$1
temp=${file}.tmp

jq '
    ["ietf-system:system", "infix-system:software", "check-update", "update-url"] as $old
  | ["ietf-system:system", "infix-system:software", "update-url"]                as $new
  | if getpath($old) != null
    then setpath($new; getpath($old)) | delpaths([$old])
    else . end
' "$file" > "$temp" && mv "$temp" "$file"
