---
title: "services/api Dockerfile"
source_path: "services/api/Dockerfile"
resource_kind: "Dockerfile"
---
FROM node:18
RUN npm ci
CMD ["node", "server.js"]
