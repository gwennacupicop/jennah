#!/usr/bin/env python3
"""
Phase 5: Generate Test Data for GCP Batch Processing
Generates scalable test files for distributed processing demo
"""

import sys
import argparse
from pathlib import Path

def generate_test_file(filename: str, num_lines: int = 5000000):
    """Generate test file with specified number of lines"""
    print(f"Generating {num_lines:,} line test file: {filename}")
    
    batch_size = 100000
    bytes_written = 0
    
    try:
        with open(filename, 'w') as f:
            for i in range(num_lines):
                line = f"Line {i}: Sample data for GCP Batch distributed processing with metrics collection\n"
                f.write(line)
                bytes_written += len(line.encode('utf-8'))
                
                # Progress indicator
                if (i + 1) % batch_size == 0:
                    mb_written = bytes_written / (1024 * 1024)
                    pct = (i + 1) / num_lines * 100
                    print(f"  [{pct:5.1f}%] {mb_written:6.1f} MB written")
        
        # Final stats
        file_size = Path(filename).stat().st_size
        print(f"\n✓ File created successfully")
        print(f"  Filename: {filename}")
        print(f"  Lines: {num_lines:,}")
        print(f"  Size: {file_size:,} bytes ({file_size / (1024**2):.2f} MB)")
        print(f"\nNext: Upload to GCS")
        print(f"  gsutil cp {filename} gs://YOUR-BUCKET/input/")
        
    except Exception as e:
        print(f"✗ Error generating file: {e}")
        sys.exit(1)

def main():
    parser = argparse.ArgumentParser(
        description="Generate test data for Phase 5 GCP Batch processing"
    )
    parser.add_argument(
        '-o', '--output',
        default='test-data-100mb.txt',
        help='Output filename (default: test-data-100mb.txt)'
    )
    parser.add_argument(
        '-l', '--lines',
        type=int,
        default=5000000,
        help='Number of lines to generate (default: 5000000)'
    )
    parser.add_argument(
        '-s', '--size',
        choices=['small', 'medium', 'large'],
        help='Preset sizes: small=1M lines, medium=5M lines, large=10M lines'
    )
    
    args = parser.parse_args()
    
    # Apply preset sizes
    if args.size == 'small':
        num_lines = 1000000
        args.output = 'test-data-50mb.txt'
    elif args.size == 'medium':
        num_lines = 5000000
        args.output = 'test-data-100mb.txt'
    elif args.size == 'large':
        num_lines = 10000000
        args.output = 'test-data-500mb.txt'
    else:
        num_lines = args.lines
    
    generate_test_file(args.output, num_lines)

if __name__ == '__main__':
    main()
