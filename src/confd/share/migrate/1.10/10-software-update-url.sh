#!/bin/sh
# Move software/check-update/update-url to the shared software/update-url,
# convert it from a repository URL to an RSS/Atom release feed, and ensure
# every config that has a software container names a feed.
#
# The update source was lifted out of the check-update container so that
# check-update and unattended-update share a single setting, and the latest
# version is now read from a release feed instead of the GitHub REST API.
#
# update-url has no model default any more: the release channel a product
# ships with belongs in its factory-config, not in the YANG.  It is mandatory
# instead, so a config that enabled check-update without ever setting the URL
# -- previously fine, the default covered it -- would no longer validate.
# Such configs inherit the feed from this unit's factory-config, which is the
# same value the model default used to supply on a stock build, and the
# vendor's own channel on a br2-external.
#
# Configs with no software container at all are left untouched; the container
# carries presence, so its absence stays valid.

file=$1
temp=${file}.tmp
factory=${FACTORY_CONFIG:-/etc/factory-config.cfg}

# The feed this unit was built with, empty if factory-config does not name one.
url=$(jq -r 'getpath(["ietf-system:system", "infix-system:software", "update-url"])
             // empty' "$factory" 2>/dev/null)

jq --arg url "$url" '
    ["ietf-system:system", "infix-system:software", "check-update", "update-url"] as $old
  | ["ietf-system:system", "infix-system:software", "update-url"]                as $new
  | ["ietf-system:system", "infix-system:software"]                              as $sw
  | (if getpath($old) != null
     then setpath($new; getpath($old)) | delpaths([$old])
     else . end)
  | (getpath($new) as $u
     | if ($u | type) == "string" and (($u | endswith(".atom")) | not)
       then setpath($new; ($u | sub("/+$"; "")) + "/releases.atom")
       else . end)
  | (if getpath($sw) != null and getpath($new) == null and $url != ""
     then setpath($new; $url)
     else . end)
' "$file" > "$temp" && mv "$temp" "$file"
