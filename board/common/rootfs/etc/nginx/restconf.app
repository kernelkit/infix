# /telemetry/optics is for streaming (not used atm)
location ~ ^/(restconf|yang|.well-known)/ {
    # Rousette block percent encoding of : in the path
    # Python requests enforce percent encoding of :
    # So we allow both.
    rewrite_by_lua_block {
       local req = ngx.var.request_uri

       local path, query = req:match("([^?]*)(%??.*)")
       local new_path = path:gsub("%%3A", ":")

       -- If path changed, update the internal request
       if new_path ~= path then
          ngx.req.set_uri(new_path)

          -- If there was a query string, we need to set that too
          if query and query ~= "" then
             -- Remove leading "?" before setting URI args
             local args_str = query:sub(2)
             ngx.req.set_uri_args(args_str)
          end
        end
    }
    grpc_pass grpc://[::1]:10080;
    grpc_set_header Host $host;
    grpc_set_header X-Real-IP $remote_addr;
    grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    grpc_set_header X-Forwarded-Proto $scheme;
}
