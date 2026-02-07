# Stage 1: Build - Process themes and create tar.gz archives
FROM alpine:3.19 AS builder

# Install required tools
RUN apk add --no-cache tar gzip jq bash

# Create working directory
WORKDIR /build

# Copy all themes
COPY . /build/

# Create output directory
RUN mkdir -p /output

# Process each theme directory
RUN for theme_dir in */; do \
    if [ -f "${theme_dir}tokens.json" ]; then \
        theme_name=$(basename "$theme_dir"); \
        echo "Processing theme: $theme_name"; \
        tar -czf "/output/${theme_name}.tar.gz" \
            -C "/build" \
            --exclude="*.DS_Store" \
            --exclude="__MACOSX" \
            "$theme_name"; \
    fi; \
done

# Generate index.json
RUN echo '{"themes":[]}' > /output/index.json && \
    for theme_dir in */; do \
        if [ -f "${theme_dir}tokens.json" ]; then \
            theme_name=$(basename "$theme_dir"); \
            tokens_file="${theme_dir}tokens.json"; \
            \
            name=$(jq -r '.theme_name // "'$theme_name'"' "$tokens_file"); \
            version=$(jq -r '.theme_version // "1.0.0"' "$tokens_file"); \
            description=$(jq -r '.theme_description // "No description available"' "$tokens_file"); \
            compiler_version=$(jq -r '.compiler_version // ">=0.1.0"' "$tokens_file"); \
            \
            size=$(stat -c%s "/output/${theme_name}.tar.gz" 2>/dev/null || stat -f%z "/output/${theme_name}.tar.gz"); \
            \
            components="[]"; \
            if [ -d "${theme_dir}src/mdx-components" ]; then \
                components=$(cd "${theme_dir}src/mdx-components" && \
                    ls *.html 2>/dev/null | sed 's/\.html$//' | \
                    awk 'BEGIN{printf "["} {gsub(/-/," "); for(i=1;i<=NF;i++){$i=toupper(substr($i,1,1)) tolower(substr($i,2))}} {gsub(/ /,""); printf "%s\"%s\"", (NR>1?",":""), $0} END{printf "]"}' || echo "[]"); \
            fi; \
            \
            theme_obj=$(jq -n \
                --arg name "$name" \
                --arg id "$theme_name" \
                --arg version "$version" \
                --arg description "$description" \
                --arg compiler_version "$compiler_version" \
                --arg download_url "/${theme_name}.tar.gz" \
                --argjson size_bytes "$size" \
                --argjson components "$components" \
                '{name: $name, id: $id, version: $version, description: $description, compiler_version: $compiler_version, download_url: $download_url, size_bytes: $size_bytes, components: $components}'); \
            \
            jq ".themes += [$theme_obj]" /output/index.json > /output/index.json.tmp && \
            mv /output/index.json.tmp /output/index.json; \
        fi; \
    done

# Pretty-print the final index
RUN jq '.' /output/index.json > /output/index.json.tmp && \
    mv /output/index.json.tmp /output/index.json

# Stage 2: Serve - Nginx to serve themes
FROM nginx:alpine

# Copy themes and index from builder
COPY --from=builder /output /usr/share/nginx/html/

# Create custom nginx config
COPY <<'EOF' /etc/nginx/conf.d/default.conf
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;

    # Serve index.json at root
    location = / {
        default_type application/json;
        try_files /index.json =404;
    }

    # Serve theme archives
    location ~ \.tar\.gz$ {
        add_header Content-Type application/gzip;
        add_header Content-Disposition "attachment";
    }

    # Cache static files
    location / {
        add_header Cache-Control "public, max-age=3600";
    }
}
EOF

# Expose port
EXPOSE 80

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
