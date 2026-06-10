#!/bin/bash
set -e

# Vercel Deployment Helper Script
# This script helps agents prepare for Vercel deployment

echo "Vercel Deployment Helper" >&2
echo "=======================" >&2

# Check for package.json
if [ -f "package.json" ]; then
    echo "✓ package.json found" >&2
    
    # Detect framework from package.json
    if grep -q '"next"' package.json || grep -q '"@vercel/next"' package.json; then
        echo "Framework: Next.js" >&2
        echo "Build command: npm run build" >&2
        echo "Output directory: .next" >&2
    elif grep -q '"react-scripts"' package.json; then
        echo "Framework: Create React App" >&2
        echo "Build command: npm run build" >&2
        echo "Output directory: build" >&2
    elif grep -q '"vite"' package.json; then
        echo "Framework: Vite" >&2
        echo "Build command: npm run build" >&2
        echo "Output directory: dist" >&2
    else
        echo "Framework: Node.js/Unknown" >&2
        echo "Build command: npm run build (assumed)" >&2
    fi
else
    echo "⚠️ No package.json found (static site?)" >&2
fi

# Check for vercel.json
if [ -f "vercel.json" ]; then
    echo "✓ vercel.json configuration found" >&2
else
    echo "ℹ️ No vercel.json (will use defaults)" >&2
fi

# Check for .env files
if [ -f ".env" ] || [ -f ".env.local" ] || [ -f ".env.production" ]; then
    echo "⚠️ .env files found - remember to set environment variables in Vercel dashboard" >&2
fi

echo ""
echo "Deployment Commands:" >&2
echo "1. Install Vercel CLI: npm i -g vercel" >&2
echo "2. Deploy: vercel --prod" >&2
echo "3. For preview: vercel" >&2
