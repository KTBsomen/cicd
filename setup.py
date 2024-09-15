
from setuptools import setup, find_packages

with open("README.md", "r", encoding="utf-8") as fh:
    long_description = fh.read()

setup(
    name="cicd",
    version="0.1.0",
    author="somen das",
    author_email="KTBsomen@gmail.com",
    description="An easy replacement for docker and all kind of ci cd tool based on shell script",
    long_description=long_description,
    long_description_content_type="text/markdown",
    url="https://github.com/KTBsomen/cicd",
    packages=find_packages(),
    classifiers=[
        "Programming Language :: Python :: 3",
        "Operating System :: Linux",
    ],
    python_requires=">=3.6",
    install_requires=[
        # Add your project dependencies here, e.g.:
        # "requests>=2.25.1",
        # "numpy>=1.20.0",
        "pymongo",
        "flask",
        "flask-socketio"
        
    ],
    entry_points={
        "console_scripts": [
            # Add any command-line scripts here, e.g.:
            # "your_script_name=your_package.module:function",
            "cicd=main:main",
        ],
    },
)
