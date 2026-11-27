location = /netbrowse {
    return 301 /netbrowse/;
}
location /netbrowse/ {
    rewrite ^/netbrowse/(.*)$ /$1 break;
    include /etc/nginx/netbrowse.conf;
}
