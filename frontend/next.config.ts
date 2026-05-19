import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  allowedDevOrigins: ['*.ngrok-free.app', '*.ngrok-free.dev', '*.ngrok.io'],
  async rewrites() {
    const target = process.env.API_INTERNAL_URL || 'http://localhost:8080'
    return [
      { source: '/api/:path*',     destination: `${target}/api/:path*` },
      { source: '/uploads/:path*', destination: `${target}/uploads/:path*` },
    ]
  },
  images: {
    remotePatterns: [
      // Storage interno — dev (Go backend local)
      {
        protocol: 'http',
        hostname: 'localhost',
        port: '8080',
        pathname: '/uploads/**',
      },
      // Storage interno — prod (AWS S3, us-east-1 e região global)
      {
        protocol: 'https',
        hostname: '*.s3.amazonaws.com',
        pathname: '/**',
      },
      // Storage interno — prod (AWS S3, sa-east-1 regional path-style)
      {
        protocol: 'https',
        hostname: '*.s3.sa-east-1.amazonaws.com',
        pathname: '/**',
      },
      // Transição: URLs no banco ainda apontam para TCGDex CDN.
      // Remover após re-import completo com --download-images.
      {
        protocol: 'https',
        hostname: 'assets.tcgdex.net',
      },
      // Transição: logos de promo vindos do pokemontcg.io antes do download.
      // Remover após re-import completo com --download-images.
      {
        protocol: 'https',
        hostname: 'images.pokemontcg.io',
      },
      // Transição: imagens vindas do Scrydex antes do download para S3.
      // Remover após re-import completo com --skip-images=false.
      {
        protocol: 'https',
        hostname: 'images.scrydex.com',
      },
      // Transição: imagens vindas do Scrydex antes do download para S3.
      // Remover após re-import completo com --skip-images=false.
      {
        protocol: 'https',
        hostname: 'images.scrydex.com',
      },
    ],
  },
};

export default nextConfig;
