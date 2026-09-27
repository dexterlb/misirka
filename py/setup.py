from setuptools import setup

setup(name='misirka',
      version='0.1.0',
      description='Realtime RPC',
      url='https://github.com/dexterlb/misirka',
      packages=['misirka', 'misirka.srv_wrapper'],
      entry_points={
          'console_scripts': ['misirka-test-toy=misirka.srv_wrapper.test_toy:main'],
      }
)
