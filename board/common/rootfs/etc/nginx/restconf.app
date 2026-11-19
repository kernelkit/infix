# /telemetry/optics is for streaming (not used atm)
location ~ ^/(restconf|yang|.well-known)/ {
    include /etc/nginx/restconf-access.conf;
    include /etc/nginx/restconf-rate-limit.conf;
    grpc_pass grpc://[::1]:10080;
    grpc_set_header Host $host;
    grpc_set_header X-Real-IP $remote_addr;
    grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    grpc_set_header X-Forwarded-Proto $scheme;

    # A little longer than the RESTCONF server waits, see /etc/default/confd
    grpc_read_timeout 130s;

    # Any error_page here stops the inherited one, 599 is never sent
    error_page 599 /50x.html;
}
