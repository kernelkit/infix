# /telemetry/optics is for streaming (not used atm)
location ~ ^/(restconf|yang|.well-known)/ {
    include /etc/nginx/restconf-access.conf;
    include /etc/nginx/restconf-rate-limit.conf;

    # A little longer than the RESTCONF server waits, see /etc/default/confd
    grpc_read_timeout 130s;
    include /etc/nginx/restconf-proxy.conf;
}

# Event streams stay open, rousette sends a keep-alive every 55 s
location /streams/ {
    include /etc/nginx/restconf-access.conf;
    include /etc/nginx/restconf-rate-limit.conf;
    limit_conn restconf_streams 16;
    limit_conn_status 429;
    grpc_read_timeout 120s;
    include /etc/nginx/restconf-proxy.conf;
}
