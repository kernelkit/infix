# /telemetry/optics is for streaming (not used atm)
location ~ ^/(restconf|yang|.well-known)/ {
    grpc_pass grpc://[::1]:10080;
    grpc_set_header Host $host;
    grpc_set_header X-Real-IP $remote_addr;
    grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    grpc_set_header X-Forwarded-Proto $scheme;
}
