# /telemetry/optics is for streaming (not used atm)
location ~ ^/(restconf|yang|.well-known)/ {
    include /etc/nginx/restconf-access.conf;
    limit_req zone=restconf burst=20 nodelay;
    limit_req_status 429;
    grpc_pass grpc://[::1]:10080;
    grpc_set_header Host $host;
    grpc_set_header X-Real-IP $remote_addr;
    grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    grpc_set_header X-Forwarded-Proto $scheme;
}
