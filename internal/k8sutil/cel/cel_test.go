package cel

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CEL Expression Builder", func() {
	Describe("BuildExpr", func() {
		DescribeTable("should build correct CEL expressions for bool values",
			func(op ComparisonOperator, value interface{}, expected string, expectError bool) {
				result, err := BuildExpr("device.attributes[\"nvidia.com\"].test_bool", op, value, TypeBool)
				if expectError {
					Expect(err).To(HaveOccurred())
					Expect(result).To(Equal(""))
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(expected))
				}
			},
			Entry("equal true", OpEqual, true, "device.attributes[\"nvidia.com\"].test_bool == true", false),
			Entry("equal false", OpEqual, false, "device.attributes[\"nvidia.com\"].test_bool == false", false),
			Entry("not equal true", OpNotEqual, true, "device.attributes[\"nvidia.com\"].test_bool != true", false),
			Entry("not equal false", OpNotEqual, false, "device.attributes[\"nvidia.com\"].test_bool != false", false),
			Entry("invalid operator", ComparisonOperator("unknown"), true, "", true),
		)

		DescribeTable("should build correct CEL expressions for int values",
			func(op ComparisonOperator, value interface{}, expected string, expectError bool) {
				result, err := BuildExpr("device.attributes[\"nvidia.com\"].test_int", op, value, TypeInt)
				if expectError {
					Expect(err).To(HaveOccurred())
					Expect(result).To(Equal(""))
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(expected))
				}
			},
			Entry("equal positive", OpEqual, 42, "device.attributes[\"nvidia.com\"].test_int == 42", false),
			Entry("equal negative", OpEqual, -100, "device.attributes[\"nvidia.com\"].test_int == -100", false),
			Entry("equal zero", OpEqual, 0, "device.attributes[\"nvidia.com\"].test_int == 0", false),
			Entry("not equal positive", OpNotEqual, 42, "device.attributes[\"nvidia.com\"].test_int != 42", false),
			Entry("not equal negative", OpNotEqual, -100, "device.attributes[\"nvidia.com\"].test_int != -100", false),
			Entry("greater than", OpGreater, 10, "device.attributes[\"nvidia.com\"].test_int > 10", false),
			Entry("greater than or equal", OpGreaterOrEqual, 10, "device.attributes[\"nvidia.com\"].test_int >= 10", false),
			Entry("less than", OpLess, 10, "device.attributes[\"nvidia.com\"].test_int < 10", false),
			Entry("less than or equal", OpLessOrEqual, 10, "device.attributes[\"nvidia.com\"].test_int <= 10", false),
			Entry("invalid operator", ComparisonOperator("unknown"), 42, "", true),
		)

		DescribeTable("should build correct CEL expressions for string values",
			func(op ComparisonOperator, value interface{}, expected string, expectError bool) {
				result, err := BuildExpr("device.attributes[\"nvidia.com\"].test_string", op, value, TypeString)
				if expectError {
					Expect(err).To(HaveOccurred())
					Expect(result).To(Equal(""))
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(expected))
				}
			},
			Entry("equal simple", OpEqual, "hello", "device.attributes[\"nvidia.com\"].test_string == \"hello\"", false),
			Entry("equal with spaces", OpEqual, "hello world", "device.attributes[\"nvidia.com\"].test_string == \"hello world\"", false),
			Entry("equal empty", OpEqual, "", "device.attributes[\"nvidia.com\"].test_string == \"\"", false),
			Entry("equal with quotes", OpEqual, "value with \"quotes\"", "device.attributes[\"nvidia.com\"].test_string == \"value with \\\"quotes\\\"\"", false),
			Entry("not equal simple", OpNotEqual, "hello", "device.attributes[\"nvidia.com\"].test_string != \"hello\"", false),
			Entry("not equal with spaces", OpNotEqual, "hello world", "device.attributes[\"nvidia.com\"].test_string != \"hello world\"", false),
			Entry("invalid operator", ComparisonOperator("unknown"), "hello", "", true),
		)

		DescribeTable("should build correct CEL expressions for semver values",
			func(op ComparisonOperator, value interface{}, expected string, expectError bool) {
				result, err := BuildExpr("device.attributes[\"nvidia.com\"].test_version", op, value, TypeSemver)
				if expectError {
					Expect(err).To(HaveOccurred())
					Expect(result).To(Equal(""))
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(expected))
				}
			},
			Entry("equal simple", OpEqual, "1.2.3", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3\")) == 0", false),
			Entry("equal with pre-release", OpEqual, "1.2.3-alpha.1", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3-alpha.1\")) == 0", false),
			Entry("equal with build", OpEqual, "1.2.3+build.1", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3+build.1\")) == 0", false),
			Entry("complex semver", OpEqual, "1.2.3-alpha.1+build.123", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3-alpha.1+build.123\")) == 0", false),
			Entry("not equal simple", OpNotEqual, "1.2.3", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3\")) != 0", false),
			Entry("greater than", OpGreater, "1.2.3", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3\")) > 0", false),
			Entry("greater than or equal", OpGreaterOrEqual, "1.2.3", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3\")) >= 0", false),
			Entry("less than", OpLess, "1.2.3", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3\")) < 0", false),
			Entry("less than or equal", OpLessOrEqual, "1.2.3", "(device.attributes[\"nvidia.com\"].test_version).compareTo(semver(\"1.2.3\")) <= 0", false),
			Entry("invalid operator", ComparisonOperator("unknown"), "1.2.3", "", true),
		)

		DescribeTable("should build correct CEL expressions for quantity values",
			func(op ComparisonOperator, value interface{}, expected string, expectError bool) {
				result, err := BuildExpr("device.attributes[\"nvidia.com\"].test_quantity", op, value, TypeQuantity)
				if expectError {
					Expect(err).To(HaveOccurred())
					Expect(result).To(Equal(""))
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(expected))
				}
			},
			Entry("equal simple", OpEqual, ptr.To(resource.MustParse("100")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"100\")) == 0", false),
			Entry("equal with unit", OpEqual, ptr.To(resource.MustParse("100Mi")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"100Mi\")) == 0", false),
			Entry("equal decimal", OpEqual, ptr.To(resource.MustParse("1.5")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"1500m\")) == 0", false),
			Entry("not equal simple", OpNotEqual, ptr.To(resource.MustParse("100")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"100\")) != 0", false),
			Entry("greater than", OpGreater, ptr.To(resource.MustParse("100")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"100\")) > 0", false),
			Entry("greater than or equal", OpGreaterOrEqual, ptr.To(resource.MustParse("100")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"100\")) >= 0", false),
			Entry("less than", OpLess, ptr.To(resource.MustParse("100")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"100\")) < 0", false),
			Entry("less than or equal", OpLessOrEqual, ptr.To(resource.MustParse("100")), "(device.attributes[\"nvidia.com\"].test_quantity).compareTo(quantity(\"100\")) <= 0", false),
			Entry("invalid operator", ComparisonOperator("unknown"), ptr.To(resource.MustParse("100")), "", true),
		)

		DescribeTable("nil values result in error",
			func(vt ValueType) {
				_, err := BuildExpr("device.attributes[\"nvidia.com\"].test", OpEqual, nil, vt)
				Expect(err).To(HaveOccurred())
			},
			Entry("bool type", TypeBool),
			Entry("int type", TypeInt),
			Entry("string type", TypeString),
			Entry("semver type", TypeSemver),
			Entry("quantity type", TypeQuantity),
			Entry("unknown type", TypeUnknown),
		)

		DescribeTable("should validate CEL expressions",
			func(expression string, expectError bool) {
				err := ValidateExpr(expression)
				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("valid attribute expression", "device.attributes[\"nvidia.com\"].test == true", false),
			Entry("valid capacity expression", "device.capacity[\"nvidia.com\"].test == quantity(\"100\")", false),
			Entry("valid driver expression", "device.driver == \"nvidia.com\"", false),
			Entry("invalid capacity type", "device.capacity[\"nvidia.com\"].test == \"100\"", true),
			Entry("invalid driver type", "device.driver == 1", true),
			Entry("invalid expression", "device.attributes[\"nvidia.com/test\"] == true", true),
			Entry("invalid device expression", "device.foo == true", true),
			Entry("invalid attribute expression", "device.attributes == true", true),
			Entry("invalid capacity expression", "device.capacity == true", true),
			Entry("invalid device", "foo.attributes[\"nvidia.com\"].test == true", true),
		)
	})
})
