# /telemetry/optics is for streaming (not used atm)
# Proxy buffer settings for large files
proxy_buffering off;                 # Disable buffering for streaming
proxy_request_buffering off;         # Stream request body immediately
proxy_max_temp_file_size 0;         # No temp files
location ~ ^/(restconf|yang|.well-known)/ {
    client_max_body_size 200M;
    client_body_buffer_size 1M;
    grpc_pass grpc://[::1]:10080;
    grpc_set_header Host $host;
    grpc_set_header X-Real-IP $remote_addr;
    grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    grpc_set_header X-Forwarded-Proto $scheme;
}
