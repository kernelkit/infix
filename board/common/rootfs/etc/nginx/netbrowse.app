location /netbrowse/ {
    return 301 /netbrowse;
}
location /netbrowse {
    include /etc/nginx/netbrowse.conf;
}
