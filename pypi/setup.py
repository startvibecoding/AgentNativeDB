import os
from setuptools import Distribution, setup, find_packages
from setuptools.command.bdist_wheel import bdist_wheel as _bdist_wheel


class BinaryDistribution(Distribution):
    """标记为包含平台特定二进制文件的分发包"""
    def has_ext_modules(self):
        return True


class bdist_wheel(_bdist_wheel):
    """生成平台特定的 wheel 文件"""
    def finalize_options(self):
        super().finalize_options()
        self.root_is_pure = False

    def get_tag(self):
        _, _, platform_tag = super().get_tag()
        return "py3", "none", platform_tag


setup(
    packages=find_packages(where="src"),
    package_dir={"": "src"},
    package_data={
        "andb_installer": ["bin/*"],
    },
    include_package_data=True,
    distclass=BinaryDistribution,
    cmdclass={"bdist_wheel": bdist_wheel},
)
