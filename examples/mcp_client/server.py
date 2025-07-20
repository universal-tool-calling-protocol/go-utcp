#!/usr/bin/env python3
"""
Simple HTTP server that exposes tools for UTCP
"""
from flask import Flask, request, jsonify
import json

app = Flask(__name__)

@app.route('/hello', methods=['POST'])
def hello():
    data = request.get_json() or {}
    name = data.get('name', 'World')
    return jsonify({
        "result": f"Hello, {name}!"
    })

@app.route('/utcp', methods=['GET'])
def utcp_discovery():
    """UTCP discovery endpoint"""
    return jsonify({
        "provider_type": "http",
        "name": "demo_tools",
        "config": {
            "base_url": "http://localhost:8000",
            "tools": [
                {
                    "name": "hello",
                    "description": "Say hello to someone",
                    "method": "POST",
                    "path": "/hello",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "name": {
                                "type": "string",
                                "description": "Name to greet",
                                "default": "World"
                            }
                        }
                    }
                }
            ]
        }
    })

if __name__ == '__main__':
    print("Starting UTCP-compatible HTTP server on http://localhost:8000")
    app.run(host='localhost', port=8000, debug=True)