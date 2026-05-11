import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  images: {
    remotePatterns: [
      // Storage interno — dev (Go backend local)
      {
        protocol: 'http',
        hostname: 'localhost',
        port: '8080',
        pathname: '/uploads/**',
      },
      // Storage interno — prod (AWS S3)
      {
        protocol: 'https',
        hostname: '*.s3.amazonaws.com',
        pathname: '/**',
      },
      // Manter durante transição enquanto URLs no banco ainda apontam para pokemontcg.io.
      // Remover após re-import completo com --download-images.
      {
        protocol: 'https',
        hostname: 'images.pokemontcg.io',
      },
    ],
  },
};

export default nextConfig;
